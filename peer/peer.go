package peer

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mertwole/bittorent-cli/download"
	"github.com/mertwole/bittorent-cli/peer/message"
	"github.com/mertwole/bittorent-cli/pieces"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

const connectionTimeout = time.Second * 120
const keepAliveInterval = time.Second * 120
const pendingPiecesQueueLength = 16
const pieceRequestTimeout = time.Second * 120
const blockSize = 1 << 14

type Peer struct {
	info            tracker.PeerInfo
	connection      net.Conn
	availablePieces *bitfield
	chocked         bool
	pieces          *pieces.Pieces
	pendingPieces   pendingPieces
}

type pendingPieces struct {
	pendingPieces map[int]*pendingPiece
	mutex         sync.RWMutex
}

type pendingPiece struct {
	idx            int
	data           []byte
	totalBlocks    int
	blocksReceived int
	validUntil     time.Time
}

type donePiece struct {
	idx  int
	data []byte
}

func newPendingPieces() pendingPieces {
	return pendingPieces{pendingPieces: make(map[int]*pendingPiece)}
}

func (pendingPieces *pendingPieces) insertData(piece int, offset int, data []byte) (*donePiece, error) {
	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	pendingPiece, ok := pendingPieces.pendingPieces[piece]
	if !ok {
		return nil, fmt.Errorf("unexpected piece #%d received", piece)
	}

	copy(pendingPiece.data[offset:], data)
	pendingPiece.blocksReceived++

	if pendingPiece.blocksReceived == pendingPiece.totalBlocks {
		delete(pendingPieces.pendingPieces, piece)

		return &donePiece{idx: pendingPiece.idx, data: pendingPiece.data}, nil
	}

	return nil, nil
}

func (pendingPieces *pendingPieces) length() int {
	pendingPieces.mutex.RLock()
	defer pendingPieces.mutex.RUnlock()

	return len(pendingPieces.pendingPieces)
}

func (pendingPieces *pendingPieces) insert(piece int, pieceLength int) {
	blockCount := (pieceLength + blockSize - 1) / blockSize
	newPendingPiece := pendingPiece{
		idx:            piece,
		data:           make([]byte, pieceLength),
		totalBlocks:    blockCount,
		blocksReceived: 0,
		validUntil:     time.Now().Add(pieceRequestTimeout),
	}

	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	pendingPieces.pendingPieces[piece] = &newPendingPiece
}

func (pendingPieces *pendingPieces) remove(piece int) {
	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	delete(pendingPieces.pendingPieces, piece)
}

func (pendingPieces *pendingPieces) removeStale() []int {
	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	removed := make([]int, 0)
	for piece := range pendingPieces.pendingPieces {
		if pendingPieces.pendingPieces[piece].validUntil.Before(time.Now()) {
			delete(pendingPieces.pendingPieces, piece)
			removed = append(removed, piece)
		}
	}

	return removed
}

type bitfield struct {
	bitfield []byte
	mutex    sync.RWMutex
}

func (bitfield *bitfield) addPiece(piece int) {
	byteIdx := piece / 8
	bitIdx := piece % 8

	bitfield.mutex.Lock()
	defer bitfield.mutex.Unlock()

	bitfield.bitfield[byteIdx] |= 1 << (7 - bitIdx)
}

func (bitfield *bitfield) containsPiece(piece int) bool {
	byteIdx := piece / 8
	bitIdx := piece % 8

	bitfield.mutex.RLock()
	defer bitfield.mutex.RUnlock()

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
	peer.pendingPieces = newPendingPieces()

	sendKeepAliveErrors := make(chan error)
	go peer.sendKeepAlive(sendKeepAliveErrors)

	requestBlocksErrors := make(chan error)
	go peer.requestBlocks(torrent, requestBlocksErrors)

	listenErrors := make(chan error)
	go peer.listen(torrent, downloadedPieces, listenErrors)

	go peer.checkStalePieceRequests()

	return nil
}

func (peer *Peer) listen(
	torrent *torrent_info.TorrentInfo,
	downloadedPieces chan<- download.DownloadedPiece,
	errors chan<- error,
) {
	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			errors <- fmt.Errorf("failed to decode message: %w", err)
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

			donePiece, err := peer.pendingPieces.insertData(int(index), int(begin), receivedMessage.Payload[8:])
			if err != nil {
				// TODO: Process this error?.
				log.Printf("failed to insert data to the pending piece: %v", err)
			}

			if donePiece != nil {
				log.Printf("received piece #%d", index)

				sha1 := sha1.Sum(donePiece.data)
				if torrent.Pieces[index] != sha1 {
					// TODO: Gracefully process this case
					log.Fatalf(
						"received piece with invalid hash: expected %s, got %s",
						hex.EncodeToString(torrent.Pieces[index][:]),
						hex.EncodeToString(sha1[:]),
					)
				} else {
					globalOffset := int(index) * torrent.PieceLength
					downloadedPieces <- download.DownloadedPiece{Offset: globalOffset, Data: donePiece.data}

					if !peer.pieces.CheckStateAndChange(int(index), pieces.Pending, pieces.Downloaded) {
						log.Panicf(
							"Piece is in unexpected state. Expected %v, got %v",
							pieces.Pending,
							peer.pieces.GetState(int(index)),
						)
					}
				}
			}
		case message.Cancel:
			// TODO
		}
	}
}

func (peer *Peer) requestBlocks(
	torrent *torrent_info.TorrentInfo,
	errors chan<- error,
) {
Outer:
	for {
		if peer.chocked {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		for pieceIdx := range len(torrent.Pieces) {
			if peer.availablePieces == nil || !peer.availablePieces.containsPiece(pieceIdx) {
				continue
			}

			if peer.pendingPieces.length() >= pendingPiecesQueueLength {
				time.Sleep(time.Millisecond * 100)
				continue
			}

			if !peer.pieces.CheckStateAndChange(pieceIdx, pieces.NotDownloaded, pieces.Pending) {
				continue
			}

			log.Printf("requesting piece #%d", pieceIdx)

			pieceLength := min(torrent.PieceLength, torrent.TotalLength-pieceIdx*torrent.PieceLength)
			blockCount := (pieceLength + blockSize - 1) / blockSize

			peer.pendingPieces.insert(pieceIdx, pieceLength)

			for block := range blockCount {
				length := min(blockSize, pieceLength-block*blockSize)

				messagePayload := make([]byte, 12)
				binary.BigEndian.PutUint32(messagePayload[:4], uint32(pieceIdx))         // index
				binary.BigEndian.PutUint32(messagePayload[4:8], uint32(block*blockSize)) // begin
				binary.BigEndian.PutUint32(messagePayload[8:12], uint32(length))         // length

				request := (&message.Message{ID: message.Request, Payload: messagePayload}).Encode()
				_, err := peer.connection.Write(request)
				if err != nil {
					peer.pendingPieces.remove(pieceIdx)
					errors <- fmt.Errorf("error sending piece request: %w", err)
					break Outer
				}
			}
		}
	}
}

func (peer *Peer) checkStalePieceRequests() {
	for {
		time.Sleep(pieceRequestTimeout / 10)

		stalePieces := peer.pendingPieces.removeStale()
		for _, stale := range stalePieces {
			peer.pieces.CheckStateAndChange(stale, pieces.Pending, pieces.NotDownloaded)
		}
	}
}

func (peer *Peer) sendKeepAlive(errors chan<- error) {
	for {
		time.Sleep(keepAliveInterval)

		message := message.EncodeKeepAlive()

		_, err := peer.connection.Write(message)
		if err != nil {
			errors <- fmt.Errorf("error sending keep-alive message: %w", err)
			break
		}

		log.Printf("sent keep-alive message")
	}
}
