package pending_pieces

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mertwole/bittorrent-cli/download/peer/constants"
)

type PendingPieces struct {
	pendingPieces map[int]*pendingPiece
	mutex         sync.RWMutex
}

type pendingPiece struct {
	idx           int
	data          []byte
	pendingBlocks []PendingBlock
	validUntil    time.Time
}

type PendingBlock struct {
	Offset int
	Length int
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

	blockRemoved := false
	for blockIdx, block := range pendingPiece.pendingBlocks {
		if block.Offset == offset && block.Length == len(data) {
			pendingPiece.pendingBlocks = slices.Delete(pendingPiece.pendingBlocks, blockIdx, blockIdx+1)
			blockRemoved = true
		}
	}

	if !blockRemoved {
		return nil, fmt.Errorf("unknown block with offset %d and length %d", offset, len(data))
	}

	copy(pendingPiece.data[offset:], data)

	if len(pendingPiece.pendingBlocks) == 0 {
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

func (pendingPieces *PendingPieces) GetIndexes() []int {
	pendingPieces.mutex.RLock()
	defer pendingPieces.mutex.RUnlock()

	indexes := make([]int, 0)
	for pieceIdx := range pendingPieces.pendingPieces {
		indexes = append(indexes, pieceIdx)
	}

	return indexes
}

func (pendingPieces *PendingPieces) GetPendingBlocksForPiece(piece int) []PendingBlock {
	pendingPieces.mutex.RLock()
	defer pendingPieces.mutex.RUnlock()

	pendingPiece, found := pendingPieces.pendingPieces[piece]
	if !found {
		return make([]PendingBlock, 0)
	} else {
		return pendingPiece.pendingBlocks
	}
}

func (pendingPieces *PendingPieces) ContainsPiece(piece int) bool {
	pendingPieces.mutex.RLock()
	defer pendingPieces.mutex.RUnlock()

	_, contains := pendingPieces.pendingPieces[piece]

	return contains
}

func (pendingPieces *PendingPieces) Insert(piece int, pieceLength int) {
	pendingBlocks := make([]PendingBlock, 0)

	blockCount := (pieceLength + constants.BlockSize - 1) / constants.BlockSize
	for block := range blockCount {
		length := min(constants.BlockSize, pieceLength-block*constants.BlockSize)
		offset := block * constants.BlockSize

		pendingBlocks = append(pendingBlocks, PendingBlock{Length: length, Offset: offset})
	}

	newPendingPiece := pendingPiece{
		idx:           piece,
		data:          make([]byte, pieceLength),
		pendingBlocks: pendingBlocks,
		validUntil:    time.Now().Add(constants.PieceRequestTimeout),
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
