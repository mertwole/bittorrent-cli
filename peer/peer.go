package peer

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/mertwole/bittorrent-cli/bitfield"
	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer/message"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
)

const connectionTimeout = time.Second * 120
const keepAliveInterval = time.Second * 120
const pendingPiecesQueueLength = 5
const pieceRequestTimeout = time.Second * 120
const blockSize = 1 << 14
const requestedPiecesPopInterval = time.Millisecond * 100

type Peer struct {
	info            tracker.PeerInfo
	connection      net.Conn
	availablePieces *bitfield.ConcurrentBitfield
	chocked         bool
	pieces          *pieces.Pieces
	pendingPieces   pendingPieces
	requestedPieces requestedPieces
}

// TODO: Upper bound on this?
type requestedPieces struct {
	pieces []pieceRequest
	mutex  sync.RWMutex
}

type pieceRequest struct {
	piece  int
	offset int
	length int
}

func (requestedPieces *requestedPieces) addRequest(request pieceRequest) {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	if slices.Contains(requestedPieces.pieces, request) {
		return
	}

	requestedPieces.pieces = append(requestedPieces.pieces, request)
}

func (requestedPieces *requestedPieces) cancelRequest(request pieceRequest) {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	idx := slices.Index(requestedPieces.pieces, request)
	if idx != -1 {
		requestedPieces.pieces = slices.Delete(requestedPieces.pieces, idx, idx+1)
	}
}

func (requestedPieces *requestedPieces) popRequest() *pieceRequest {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	if len(requestedPieces.pieces) == 0 {
		return nil
	}

	request := requestedPieces.pieces[0]
	requestedPieces.pieces = requestedPieces.pieces[1:]

	return &request
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

func (peer *Peer) StartExchange(
	torrent *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
	downloadedPieces *download.Download,
) error {
	// TODO: Cancel goroutines when error occured and cleanup the pendingPieces.

	peer.pieces = pieces
	peer.pendingPieces = newPendingPieces()
	peer.availablePieces = bitfield.NewEmptyConcurrentBitfield(len(torrent.Pieces))

	err := peer.sendInitialMessages()
	if err != nil {
		return fmt.Errorf("failed to send initial messages: %w", err)
	}

	notifyPresentPiecesErrors := make(chan error)
	go peer.notifyPresentPieces(notifyPresentPiecesErrors)

	sendKeepAliveErrors := make(chan error)
	go peer.sendKeepAlive(sendKeepAliveErrors)

	requestBlocksErrors := make(chan error)
	go peer.requestBlocks(torrent, requestBlocksErrors)

	listenErrors := make(chan error)
	go peer.listen(torrent, downloadedPieces, listenErrors)

	uploadPiecesErrors := make(chan error)
	go peer.uploadPieces(downloadedPieces, uploadPiecesErrors)

	go peer.checkStalePieceRequests()

	select {
	case err := <-sendKeepAliveErrors:
		return err
	case err := <-requestBlocksErrors:
		return err
	case err := <-listenErrors:
		return err
	case err := <-notifyPresentPiecesErrors:
		return err
	case err := <-uploadPiecesErrors:
		return err
	}
}

func (peer *Peer) listen(
	torrent *torrent_info.TorrentInfo,
	downloadedPieces *download.Download,
	errors chan<- error,
) {
	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			errors <- fmt.Errorf("failed to decode message: %w", err)
			break
		}

		if receivedMessage == nil {
			// No message is received
			continue
		}

		switch msg := receivedMessage.(type) {
		case *message.KeepAlive:
			continue
		case *message.Choke:
			peer.chocked = true
		case *message.Unchoke:
			peer.chocked = false
		case *message.Interested:
			// TODO
		case *message.NotInterested:
			// TODO
		case *message.Have:
			peer.availablePieces.AddPiece(msg.Piece)
		case *message.Bitfield:
			peer.availablePieces = bitfield.NewConcurrentBitfield(
				msg.Bitfield,
				len(torrent.Pieces),
			)
		case *message.Request:
			request := pieceRequest{piece: msg.Piece, offset: msg.Offset, length: msg.Length}
			peer.requestedPieces.addRequest(request)
		case *message.Piece:
			donePiece, err := peer.pendingPieces.insertData(msg.Piece, msg.Offset, msg.Data)
			if err != nil {
				// TODO: Process this error?.
				log.Printf("failed to insert data to the pending piece: %v", err)
			}

			if donePiece != nil {
				log.Printf("received piece #%d", msg.Piece)

				sha1 := sha1.Sum(donePiece.data)
				var newState pieces.PieceState
				if torrent.Pieces[msg.Piece] != sha1 {
					log.Printf(
						"received piece with invalid hash: expected %s, got %s",
						hex.EncodeToString(torrent.Pieces[msg.Piece][:]),
						hex.EncodeToString(sha1[:]),
					)

					newState = pieces.NotDownloaded
				} else {
					globalOffset := int(msg.Piece) * torrent.PieceLength
					downloadedPieces.WritePiece(
						download.DownloadedPiece{Offset: globalOffset, Data: donePiece.data},
					)

					newState = pieces.Downloaded
				}

				if !peer.pieces.CheckStateAndChange(int(msg.Piece), pieces.Pending, newState) {
					log.Panicf(
						"Piece is in unexpected state. Expected %v, got %v",
						pieces.Pending,
						peer.pieces.GetState(int(msg.Piece)),
					)
				}
			}
		case *message.Cancel:
			request := pieceRequest{piece: msg.Piece, offset: msg.Offset, length: msg.Length}
			peer.requestedPieces.cancelRequest(request)
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
			if peer.availablePieces == nil || !peer.availablePieces.ContainsPiece(pieceIdx) {
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

				message := message.Request{Piece: pieceIdx, Offset: block * blockSize, Length: length}
				request := (&message).Encode()
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

func (peer *Peer) sendInitialMessages() error {
	present := peer.pieces.GetBitfield()
	if !present.IsEmpty() {
		request := (&message.Bitfield{Bitfield: present.ToBytes()}).Encode()
		_, err := peer.connection.Write(request)
		if err != nil {
			return fmt.Errorf("error sending bitfield: %w", err)
		}

		log.Printf("sent bitfield message")

		request = (&message.Unchoke{}).Encode()
		_, err = peer.connection.Write(request)
		if err != nil {
			return fmt.Errorf("error sending unchoke message: %w", err)
		}

		log.Printf("sent unchoke message")
	}

	return nil
}

func (peer *Peer) notifyPresentPieces(errors chan<- error) {
	for {
		time.Sleep(time.Second)
		// TODO: Send have messages
	}
}

func (peer *Peer) uploadPieces(downloadedPieces *download.Download, errors chan<- error) {
	for {
		requestedPiece := peer.requestedPieces.popRequest()
		if requestedPiece == nil {
			time.Sleep(requestedPiecesPopInterval)
			continue
		}

		pieceData, err := downloadedPieces.ReadPiece(requestedPiece.piece)
		if err != nil {
			errors <- fmt.Errorf("failed to read piece #%d: %w", requestedPiece.piece, err)
		}

		// TODO: Add method to partially read piece.
		block := (*pieceData)[requestedPiece.offset : requestedPiece.offset+requestedPiece.length]

		message := message.Piece{Piece: requestedPiece.piece, Offset: requestedPiece.offset, Data: block}
		_, err = peer.connection.Write(message.Encode())
		if err != nil {
			errors <- fmt.Errorf("error sending piece message: %w", err)
			break
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

		message := (&message.KeepAlive{}).Encode()
		_, err := peer.connection.Write(message)
		if err != nil {
			errors <- fmt.Errorf("error sending keep-alive message: %w", err)
			break
		}

		log.Printf("sent keep-alive message")
	}
}
