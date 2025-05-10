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
	"github.com/mertwole/bittorent-cli/pieces"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

const connectionTimeout = time.Second * 120
const keepAliveInterval = time.Second * 120
const pendingPiecesQueueLength = 3

type Peer struct {
	info            tracker.PeerInfo
	connection      net.Conn
	availablePieces *bitfield // TODO: Mutex
	chocked         bool
	pieces          *pieces.Pieces
	// TODO: Mutex
	// TODO: check for stale pending pieces and retry download
	pendingPieces map[int]*pendingPiece
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

func (peer *Peer) GetInfo() tracker.PeerInfo {
	return peer.info
}

func (peer *Peer) Connect(info *tracker.PeerInfo) error {
	peer.info = *info
	peer.chocked = true

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
	pieces *pieces.Pieces,
	downloadedPieces chan<- download.DownloadedPiece,
) error {
	peer.pieces = pieces
	peer.pendingPieces = make(map[int]*pendingPiece)

	go peer.sendKeepAlive()

	go peer.requestBlocks(torrent)
	go peer.listen(torrent, downloadedPieces)

	return nil
}

func (peer *Peer) listen(
	torrent *torrent_info.TorrentInfo,
	downloadedPieces chan<- download.DownloadedPiece,
) {
	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			// TODO: Reconnect in this case.
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
			index := binary.BigEndian.Uint32(receivedMessage.Payload[:4])
			begin := binary.BigEndian.Uint32(receivedMessage.Payload[4:8])

			pendingPiece, ok := peer.pendingPieces[int(index)]
			if !ok {
				log.Printf("unexpected piece #%d received", index)
				continue
			}

			copy(pendingPiece.data[begin:], receivedMessage.Payload[8:])
			pendingPiece.blocksReceived++

			if pendingPiece.blocksReceived == pendingPiece.totalBlocks {
				log.Printf("received piece #%d", index)

				sha1 := sha1.Sum(pendingPiece.data)
				if torrent.Pieces[index] != sha1 {
					// TODO: Gracefully process this case
					log.Fatalf(
						"received piece with invalid hash: expected %s, got %s",
						hex.EncodeToString(torrent.Pieces[index][:]),
						hex.EncodeToString(sha1[:]),
					)
				} else {
					globalOffset := int(index) * torrent.PieceLength
					downloadedPieces <- download.DownloadedPiece{Offset: globalOffset, Data: pendingPiece.data}

					if !peer.pieces.CheckStateAndChange(int(index), pieces.Pending, pieces.Downloaded) {
						log.Panicf(
							"Piece is in unexpected state. Expected %v, got %v",
							pieces.Pending,
							peer.pieces.GetState(int(index)),
						)
					}

					delete(peer.pendingPieces, int(index))
				}
			}
		case message.Cancel:
			// TODO
		}
	}
}

const blockSize = 1 << 14

func (peer *Peer) requestBlocks(
	torrent *torrent_info.TorrentInfo,
) {
	for {
		if peer.chocked {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		for pieceIdx := range len(torrent.Pieces) {
			if peer.availablePieces == nil || !peer.availablePieces.containsPiece(pieceIdx) {
				continue
			}

			if !peer.pieces.CheckStateAndChange(pieceIdx, pieces.NotDownloaded, pieces.Pending) {
				continue
			}

			if len(peer.pendingPieces) >= pendingPiecesQueueLength {
				time.Sleep(time.Millisecond * 100)
				continue
			}

			log.Printf("requesting piece #%d", pieceIdx)

			pieceLength := min(torrent.PieceLength, torrent.TotalLength-pieceIdx*torrent.PieceLength)
			blockCount := (pieceLength + blockSize - 1) / blockSize

			peer.pendingPieces[pieceIdx] = &pendingPiece{
				idx:            pieceIdx,
				data:           make([]byte, pieceLength),
				totalBlocks:    blockCount,
				blocksReceived: 0,
			}

			for block := range blockCount {
				length := min(blockSize, pieceLength-block*blockSize)

				messagePayload := make([]byte, 12)
				binary.BigEndian.PutUint32(messagePayload[:4], uint32(pieceIdx))         // index
				binary.BigEndian.PutUint32(messagePayload[4:8], uint32(block*blockSize)) // begin
				binary.BigEndian.PutUint32(messagePayload[8:12], uint32(length))         // length

				request := (&message.Message{ID: message.Request, Payload: messagePayload}).Encode()
				_, err := peer.connection.Write(request)
				if err != nil {
					// TODO: Reconnect in this case.
					log.Printf("error sending piece request: %v", err)
				}
			}
		}
	}
}

func (peer *Peer) sendKeepAlive() {
	for {
		time.Sleep(keepAliveInterval)

		message := message.EncodeKeepAlive()

		_, err := peer.connection.Write(message)
		if err != nil {
			// TODO: Reconnect in this case.
			log.Printf("error sending keep-alive message: %v", err)
		}

		log.Printf("sent keep-alive message")
	}
}
