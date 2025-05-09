package peer

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
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

const connectionTimeout = time.Second * 120
const keepAliveInterval = time.Second * 120
const pendingPiecesQueueLength = 16

type Peer struct {
	info            tracker.PeerInfo
	connection      net.Conn
	availablePieces *bitfield
	chocked         bool
	pendingPieces   map[int]*pendingPiece // TODO: check for stale pending pieces and retry download.
}

type pendingPiece struct {
	idx            int
	data           []byte
	totalBlocks    int
	blocksReceived int
}

type bitfield struct {
	bitfield []byte
}

func (bitfield *bitfield) addPiece(piece int) {
	byteIdx := piece / 8
	bitIdx := piece % 8

	bitfield.bitfield[byteIdx] |= 1 << (7 - bitIdx)
}

func (bitfield *bitfield) containsPiece(piece int) bool {
	byteIdx := piece / 8
	bitIdx := piece % 8

	return bitfield.bitfield[byteIdx]&(1<<(7-bitIdx)) != 0
}

func (peer *Peer) Connect(info *tracker.PeerInfo) error {
	peer.info = *info
	peer.chocked = true
	peer.pendingPieces = make(map[int]*pendingPiece)

	conn, err := net.DialTimeout("tcp", info.IP.String()+":"+strconv.Itoa(int(info.Port)), connectionTimeout)
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

func (peer *Peer) StartDownload(
	torrent *torrent_info.TorrentInfo,
	requestedPieces chan int,
	downloadedPieces chan<- download.DownloadedPiece,
) error {
	go peer.sendKeepAlive()

	pendingPieces := make(chan pendingPiece, pendingPiecesQueueLength)

	go peer.requestBlocks(torrent, requestedPieces, pendingPieces)
	go peer.listen(torrent, downloadedPieces, pendingPieces)

	return nil
}

func (peer *Peer) listen(
	torrent *torrent_info.TorrentInfo,
	downloadedPieces chan<- download.DownloadedPiece,
	pendingPieces <-chan pendingPiece,
) {
	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			log.Printf("failed to decode message from peer %+v: %v", peer.info, err)
			break
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
			havePiece := binary.BigEndian.Uint32(receivedMessage.Payload[:4])
			peer.availablePieces.addPiece(int(havePiece))
		case message.Bitfield:
			peer.availablePieces = &bitfield{bitfield: receivedMessage.Payload}
		case message.Request:
			// TODO
		case message.Piece:
		Outer:
			for {
				select {
				case newPendingPiece := <-pendingPieces:
					peer.pendingPieces[newPendingPiece.idx] = &newPendingPiece
				default:
					break Outer
				}
			}

			index := binary.BigEndian.Uint32(receivedMessage.Payload[:4])
			begin := binary.BigEndian.Uint32(receivedMessage.Payload[4:8])

			pendingPiece, ok := peer.pendingPieces[int(index)]
			if !ok {
				log.Fatalf("unexpected piece #%d received", index)
			}

			copy(pendingPiece.data[begin:], receivedMessage.Payload[8:])
			pendingPiece.blocksReceived++

			if pendingPiece.blocksReceived == pendingPiece.totalBlocks {
				log.Printf("received piece. #%d", index)

				sha1 := sha1.Sum(pendingPiece.data)
				if torrent.Pieces[index] != sha1 {
					log.Printf(
						"received piece with invalid hash: expected %s, got %s",
						hex.EncodeToString(torrent.Pieces[index][:]),
						hex.EncodeToString(sha1[:]),
					)
				}

				globalOffset := int(index) * torrent.PieceLength
				downloadedPieces <- download.DownloadedPiece{Offset: globalOffset, Data: pendingPiece.data}
			}
		case message.Cancel:
			// TODO
		}
	}
}

const blockSize = 1 << 14

func (peer *Peer) requestBlocks(
	torrent *torrent_info.TorrentInfo,
	requestedPieces chan int,
	pendingPieces chan<- pendingPiece,
) {
	for {
		if peer.chocked {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		piece, ok := <-requestedPieces
		if !ok {
			// TODO: Exit only when there're no pending pieces
			break
		}

		if peer.availablePieces == nil || !peer.availablePieces.containsPiece(piece) {
			requestedPieces <- piece
			continue
		}

		pieceLength := min(torrent.PieceLength, torrent.TotalLength-piece*torrent.PieceLength)
		blockCount := (pieceLength + blockSize - 1) / blockSize

		log.Printf("requesting piece #%d", piece)

		pendingPieces <- pendingPiece{
			idx:            piece,
			data:           make([]byte, pieceLength),
			totalBlocks:    blockCount,
			blocksReceived: 0,
		}

		for block := range blockCount {
			length := min(blockSize, pieceLength-block*blockSize)

			messagePayload := make([]byte, 12)
			binary.BigEndian.PutUint32(messagePayload[:4], uint32(piece))            // index
			binary.BigEndian.PutUint32(messagePayload[4:8], uint32(block*blockSize)) // begin
			binary.BigEndian.PutUint32(messagePayload[8:12], uint32(length))         // length

			request := (&message.Message{ID: message.Request, Payload: messagePayload}).Encode()
			_, err := peer.connection.Write(request)
			if err != nil {
				log.Printf("error sending piece request: %v", err)
			}

			time.Sleep(time.Millisecond * 100)
		}
	}
}

func (peer *Peer) sendKeepAlive() {
	for {
		time.Sleep(keepAliveInterval)

		message := message.EncodeKeepAlive()

		_, err := peer.connection.Write(message)
		if err != nil {
			log.Printf("error sending keep-alive message: %v", err)
		}

		log.Printf("sent keep-alive message")
	}
}
