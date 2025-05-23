package pieces

import (
	"sync"

	"github.com/mertwole/bittorrent-cli/bitfield"
)

type Pieces struct {
	mutex  sync.RWMutex
	pieces []PieceState
}

type PieceState uint8

const (
	NotDownloaded PieceState = 0
	Pending       PieceState = 1
	Downloaded    PieceState = 2
)

func New(count int, downloaded *[]int) *Pieces {
	pieces := make([]PieceState, count)
	for i := range count {
		pieces[i] = NotDownloaded
	}

	for _, downloaded := range *downloaded {
		pieces[downloaded] = Downloaded
	}

	return &Pieces{pieces: pieces}
}

func (pieces *Pieces) Length() int {
	pieces.mutex.RLock()
	defer pieces.mutex.RUnlock()

	return len(pieces.pieces)
}

func (pieces *Pieces) GetState(index int) PieceState {
	pieces.mutex.RLock()
	defer pieces.mutex.RUnlock()

	return pieces.pieces[index]
}

func (pieces *Pieces) GetBitfield() bitfield.Bitfield {
	pieces.mutex.RLock()
	defer pieces.mutex.RUnlock()

	bitfield := bitfield.NewEmptyBitfield(len(pieces.pieces))
	for i, state := range pieces.pieces {
		if state == Downloaded {
			bitfield.AddPiece(i)
		}
	}

	return bitfield
}

func (pieces *Pieces) CheckStateAndChange(index int, previousState PieceState, newState PieceState) bool {
	pieces.mutex.Lock()
	defer pieces.mutex.Unlock()

	result := false
	if pieces.pieces[index] == previousState {
		pieces.pieces[index] = newState

		result = true
	}

	return result
}
