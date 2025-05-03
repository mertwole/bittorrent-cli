package peer

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/mertwole/bittorent-cli/download"
	"github.com/mertwole/bittorent-cli/peer/message"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

type Peer struct {
	info            tracker.PeerInfo
	connection      net.Conn
	availablePieces bitfield
	chocked         bool
}

type bitfield struct {
	bitfield []byte
}

func (peer *Peer) Connect(info *tracker.PeerInfo) error {
	peer.info = *info
	peer.chocked = true

	conn, err := net.Dial("tcp", info.IP.String()+":"+strconv.Itoa(int(info.Port)))
	if err != nil {
		return fmt.Errorf("failed to establish connection with peer %s: %w", info.IP.String(), err)
	}

	peer.connection = conn

	return nil
}

func (peer *Peer) Handshake(torrent *torrent_info.TorrentInfo) error {
	handshake := Handshake{
		PeerID:   [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
		InfoHash: torrent.InfoHash,
	}
	serializedHandshake := handshake.serialize()

	_, err := peer.connection.Write(serializedHandshake)
	if err != nil {
		return fmt.Errorf("failed to send request to the peer %s: %w", peer.info.IP.String(), err)
	}

	responseHandshake, err := deserializeHandshake(peer.connection)
	if err != nil {
		return fmt.Errorf("failed to decode handshake from peer %s: %w", peer.info.IP.String(), err)
	}

	if responseHandshake.InfoHash != torrent.InfoHash {
		return fmt.Errorf(
			"invalid info hash received from the peer %s: expected %v, got %v",
			peer.info.IP.String(),
			torrent.InfoHash,
			responseHandshake.InfoHash,
		)
	}

	return nil
}

func (peer *Peer) StartDownload(torrent *torrent_info.TorrentInfo, downloadedPiecesChannel chan download.DownloadedPiece) error {
	go peer.sendKeepAlive()
	go peer.requestPieces(torrent)

	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			log.Fatal(err)
		}

		if receivedMessage == nil {
			// Keep-alive message
			continue
		}

		switch receivedMessage.ID {
		case message.Choke:
			peer.chocked = true
		case message.Unchoke:
			peer.chocked = false
		case message.Interested:
			// TODO
		case message.NotInterested:
			// TODO
		case message.Have:
			// TODO: Write bit to the bitfield
		case message.Bitfield:
			peer.availablePieces = bitfield{bitfield: receivedMessage.Payload}
		case message.Request:
			// TODO
		case message.Piece:
			index := binary.BigEndian.Uint32(receivedMessage.Payload[:4])
			begin := binary.BigEndian.Uint32(receivedMessage.Payload[4:8])

			// TODO: Check piece hash.

			log.Printf(
				"received piece. index: %d, begin: %d, length: %d",
				index,
				begin,
				len(receivedMessage.Payload)-8,
			)

			globalOffset := int(index)*torrent.PieceLength + int(begin)

			downloadedPiecesChannel <- download.DownloadedPiece{Offset: globalOffset, Data: receivedMessage.Payload[8:]}
		case message.Cancel:
			// TODO
		}
	}
}

const pieceRequest = 1 << 14

func (peer *Peer) requestPieces(torrent *torrent_info.TorrentInfo) {
	var offset int = 0

	for {
		if peer.chocked {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		piece := offset / torrent.PieceLength

		requestLength := pieceRequest
		if offset+pieceRequest >= torrent.Length {
			requestLength = torrent.Length - offset
		}

		messagePayload := make([]byte, 12)
		binary.BigEndian.PutUint32(messagePayload[:4], uint32(piece))                       // index
		binary.BigEndian.PutUint32(messagePayload[4:8], uint32(offset%torrent.PieceLength)) // begin
		binary.BigEndian.PutUint32(messagePayload[8:12], uint32(requestLength))             // length

		request := (&message.Message{ID: message.Request, Payload: messagePayload}).Encode()
		_, err := peer.connection.Write(request)
		if err != nil {
			log.Printf("error sending piece request: %v", err)
		}

		offset += requestLength

		if offset >= torrent.Length {
			return
		}

		time.Sleep(time.Millisecond * 100)
	}
}

func (peer *Peer) sendKeepAlive() {
	for {
		time.Sleep(time.Second * 10)

		message := message.EncodeKeepAlive()

		_, err := peer.connection.Write(message)
		if err != nil {
			log.Printf("error sending keep-alive message: %v", err)
		}

		log.Printf("sent keep-alive message")
	}
}
