package pending_pieces

import (
	"fmt"
	"sync"
	"time"

	"github.com/mertwole/bittorrent-cli/peer/constants"
)

type PendingPieces struct {
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

type DonePiece struct {
	Idx  int
	Data []byte
}

func NewPendingPieces() PendingPieces {
	return PendingPieces{pendingPieces: make(map[int]*pendingPiece)}
}

func (pendingPieces *PendingPieces) InsertData(piece int, offset int, data []byte) (*DonePiece, error) {
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

		return &DonePiece{Idx: pendingPiece.idx, Data: pendingPiece.data}, nil
	}

	return nil, nil
}

func (pendingPieces *PendingPieces) Length() int {
	pendingPieces.mutex.RLock()
	defer pendingPieces.mutex.RUnlock()

	return len(pendingPieces.pendingPieces)
}

func (pendingPieces *PendingPieces) Insert(piece int, pieceLength int) {
	blockCount := (pieceLength + constants.BlockSize - 1) / constants.BlockSize
	newPendingPiece := pendingPiece{
		idx:            piece,
		data:           make([]byte, pieceLength),
		totalBlocks:    blockCount,
		blocksReceived: 0,
		validUntil:     time.Now().Add(constants.PieceRequestTimeout),
	}

	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	pendingPieces.pendingPieces[piece] = &newPendingPiece
}

func (pendingPieces *PendingPieces) Remove(piece int) {
	pendingPieces.mutex.Lock()
	defer pendingPieces.mutex.Unlock()

	delete(pendingPieces.pendingPieces, piece)
}

func (pendingPieces *PendingPieces) RemoveStale() []int {
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
