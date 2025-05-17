package message

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/mertwole/bittorrent-cli/bitfield"
)

const maxPayloadLength = 100_000_000

type MessageID uint8

const (
	Choke         MessageID = 0
	Unchoke       MessageID = 1
	Interested    MessageID = 2
	NotInterested MessageID = 3
	Have          MessageID = 4
	Bitfield      MessageID = 5
	Request       MessageID = 6
	Piece         MessageID = 7
	Cancel        MessageID = 8
	unsupported   MessageID = 9
)

type Message struct {
	ID      MessageID
	Payload []byte
}

func EncodeChoke() []byte {
	return (&Message{ID: Choke, Payload: make([]byte, 0)}).encode()
}

func EncodeUnchoke() []byte {
	return (&Message{ID: Unchoke, Payload: make([]byte, 0)}).encode()
}

func EncodeInterested() []byte {
	return (&Message{ID: Interested, Payload: make([]byte, 0)}).encode()
}

func EncodeNotInterested() []byte {
	return (&Message{ID: NotInterested, Payload: make([]byte, 0)}).encode()
}

func EncodeHave(piece int) []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(piece))
	return (&Message{ID: Have, Payload: payload}).encode()
}

func EncodeBitfield(bitfield *bitfield.Bitfield) []byte {
	return (&Message{ID: Bitfield, Payload: bitfield.ToBytes()}).encode()
}

func EncodeRequest(piece int, offset int, length int) []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(offset))
	payload = binary.BigEndian.AppendUint32(payload, uint32(length))

	return (&Message{ID: Request, Payload: payload}).encode()
}

func EncodePiece(piece int, offset int, data []byte) []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(offset))
	payload = append(payload, data...)

	return (&Message{ID: Piece, Payload: payload}).encode()
}

func EncodeCancel(piece int, offset int, length int) []byte {
	payload := make([]byte, 0)
	payload = binary.BigEndian.AppendUint32(payload, uint32(piece))
	payload = binary.BigEndian.AppendUint32(payload, uint32(offset))
	payload = binary.BigEndian.AppendUint32(payload, uint32(length))

	return (&Message{ID: Cancel, Payload: payload}).encode()
}

func EncodeKeepAlive() []byte {
	return make([]byte, 4)
}

func (message *Message) encode() []byte {
	length := 1 + len(message.Payload)
	encoded := make([]byte, 4+length)

	binary.BigEndian.PutUint32(encoded[:4], uint32(length))

	encoded[4] = byte(message.ID)

	copy(encoded[5:], message.Payload)

	return encoded
}

func Decode(reader io.Reader) (*Message, error) {
	var encodedLength [4]byte
	_, err := io.ReadFull(reader, encodedLength[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	length := binary.BigEndian.Uint32(encodedLength[:])

	if length > maxPayloadLength {
		return nil, fmt.Errorf("unsupported message: exceeded maximum payload length, got messsage with length %d", length)
	}

	if length == 0 {
		return nil, nil
	}

	var encodedMessageID [1]byte
	_, err = io.ReadFull(reader, encodedMessageID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read message ID: %w", err)
	}

	var id = MessageID(encodedMessageID[0])

	if id >= unsupported {
		return nil, fmt.Errorf("invalid message ID: %d", id)
	}

	message := Message{ID: id, Payload: make([]byte, length-1)}

	_, err = io.ReadFull(reader, message.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to read message payload: %w", err)
	}

	return &message, nil
}
