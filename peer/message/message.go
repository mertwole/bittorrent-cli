package message

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"

	"github.com/mertwole/bittorrent-cli/bencode"
)

const maxPayloadLength = 100_000_000

type messageID uint8

const (
	chokeMsgID         messageID = 0
	unchokeMsgID       messageID = 1
	interestedMsgID    messageID = 2
	notInterestedMsgID messageID = 3
	haveMsgID          messageID = 4
	bitfieldMsgID      messageID = 5
	requestMsgID       messageID = 6
	pieceMsgID         messageID = 7
	cancelMsgID        messageID = 8
	extendedMsgID      messageID = 20
)

const extendedHandshakeMsgID messageID = 0

type Choke struct{}
type Unchoke struct{}
type Interested struct{}
type NotInterested struct{}
type Have struct {
	Piece int
}
type Bitfield struct {
	Bitfield []byte
}
type Request struct {
	Piece  int
	Offset int
	Length int
}
type Piece struct {
	Piece  int
	Offset int
	Data   []byte
}
type Cancel struct {
	Piece  int
	Offset int
	Length int
}
type KeepAlive struct{}

type extended struct {
	extendedMessageID messageID
	payload           []byte
}

// TODO: Add all the fields and make all optional.
type ExtendedHandshake struct {
	SupportedExtensions map[string]int `bencode:"m"`
	ClientName          string         `bencode:"v"`
	//TcpListenPort       *int 			`bencode:"p"`
	//ReceiverIPAddress   *net.IP 		`bencode:"yourip"`
	//IPv6                *net.IP 		`bencode:"ipv6"`
	//IPv4                *net.IP 		`bencode:"ipv4"`
	//RequestQueueLength  *int 			`bencode:"reqq"`
}

type Message interface {
	Encode() []byte
}

type message struct {
	ID      messageID
	Payload []byte
}

func (msg *Choke) Encode() []byte {
	return (&message{ID: chokeMsgID, Payload: make([]byte, 0)}).encode()
}

func (msg *Unchoke) Encode() []byte {
	return (&message{ID: unchokeMsgID, Payload: make([]byte, 0)}).encode()
}

func (msg *Interested) Encode() []byte {
	return (&message{ID: interestedMsgID, Payload: make([]byte, 0)}).encode()
}

func (msg *NotInterested) Encode() []byte {
	return (&message{ID: notInterestedMsgID, Payload: make([]byte, 0)}).encode()
}

func (msg *Have) Encode() []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Piece))
	return (&message{ID: haveMsgID, Payload: payload}).encode()
}

func (msg *Bitfield) Encode() []byte {
	return (&message{ID: bitfieldMsgID, Payload: msg.Bitfield}).encode()
}

func (msg *Request) Encode() []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Offset))
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Length))

	return (&message{ID: requestMsgID, Payload: payload}).encode()
}

func (msg *Piece) Encode() []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Offset))
	payload = append(payload, msg.Data...)

	return (&message{ID: pieceMsgID, Payload: payload}).encode()
}

func (msg *Cancel) Encode() []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Offset))
	payload = binary.BigEndian.AppendUint32(payload, uint32(msg.Length))

	return (&message{ID: cancelMsgID, Payload: payload}).encode()
}

func (msg *extended) Encode() []byte {
	payload := []byte{byte(msg.extendedMessageID)}
	payload = append(payload, msg.payload...)

	return (&message{ID: extendedMsgID, Payload: payload}).encode()
}

func (msg *ExtendedHandshake) Encode() []byte {
	var encodedDictionary bytes.Buffer
	err := bencode.Serialize(&encodedDictionary, *msg)
	if err != nil {
		log.Panicf("cannot encode ExtendedHandshake message: %v", err)
	}

	extendedMessage := extended{extendedMessageID: extendedHandshakeMsgID, payload: encodedDictionary.Bytes()}
	return extendedMessage.Encode()
}

func (msg *KeepAlive) Encode() []byte {
	return make([]byte, 0)
}

func (message *message) encode() []byte {
	length := 1 + len(message.Payload)
	encoded := make([]byte, 4+length)

	binary.BigEndian.PutUint32(encoded[:4], uint32(length))

	encoded[4] = byte(message.ID)

	copy(encoded[5:], message.Payload)

	return encoded
}

func Decode(reader io.Reader) (Message, error) {
	var encodedLength [4]byte
	_, err := io.ReadFull(reader, encodedLength[:])
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	length := binary.BigEndian.Uint32(encodedLength[:])

	if length > maxPayloadLength {
		return nil, fmt.Errorf("unsupported message: exceeded maximum payload length, got messsage with length %d", length)
	}

	if length == 0 {
		return &KeepAlive{}, nil
	}

	var encodedMessageID [1]byte
	_, err = io.ReadFull(reader, encodedMessageID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read message ID: %w", err)
	}

	var id = messageID(encodedMessageID[0])

	payload := make([]byte, length-1)
	_, err = io.ReadFull(reader, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to read message payload: %w", err)
	}

	// TODO: Assert message lengths.
	switch id {
	case chokeMsgID:
		return &Choke{}, nil
	case unchokeMsgID:
		return &Unchoke{}, nil
	case interestedMsgID:
		return &Interested{}, nil
	case notInterestedMsgID:
		return &NotInterested{}, nil
	case haveMsgID:
		piece := binary.BigEndian.Uint32(payload[:4])
		return &Have{Piece: int(piece)}, nil
	case bitfieldMsgID:
		return &Bitfield{Bitfield: payload}, nil
	case requestMsgID:
		piece := binary.BigEndian.Uint32(payload[:4])
		offset := binary.BigEndian.Uint32(payload[4:8])
		length := binary.BigEndian.Uint32(payload[8:12])
		return &Request{Piece: int(piece), Offset: int(offset), Length: int(length)}, nil
	case pieceMsgID:
		piece := binary.BigEndian.Uint32(payload[:4])
		offset := binary.BigEndian.Uint32(payload[4:8])
		data := payload[8:]
		return &Piece{Piece: int(piece), Offset: int(offset), Data: data}, nil
	case cancelMsgID:
		piece := binary.BigEndian.Uint32(payload[:4])
		offset := binary.BigEndian.Uint32(payload[4:8])
		length := binary.BigEndian.Uint32(payload[8:12])
		return &Cancel{Piece: int(piece), Offset: int(offset), Length: int(length)}, nil
	case extendedMsgID:
		extendedMessageID := messageID(payload[0])
		payload := payload[1:]
		extendedMessage := extended{extendedMessageID: extendedMessageID, payload: payload}
		return extendedMessage.decode()
	default:
		return nil, fmt.Errorf("invalid message ID: %d", id)
	}
}

func (extended *extended) decode() (Message, error) {
	switch extended.extendedMessageID {
	case extendedHandshakeMsgID:
		buffer := bytes.NewBuffer(extended.payload)

		decoded := ExtendedHandshake{}
		err := bencode.Deserialize(buffer, &decoded)
		if err != nil {
			return nil, fmt.Errorf("invalid extended handshake message: %w", err)
		}

		return &decoded, nil
	default:
		return nil, fmt.Errorf("invalid extended message id: %d", extended.extendedMessageID)
	}
}
