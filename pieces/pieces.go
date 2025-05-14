package pieces

import "sync"

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

func NewPieces(count int, downloaded *[]int) *Pieces {
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
	len := len(pieces.pieces)
	pieces.mutex.RUnlock()
	return len
}

func (pieces *Pieces) GetState(index int) PieceState {
	pieces.mutex.RLock()
	state := pieces.pieces[index]
	pieces.mutex.RUnlock()
	return state
}

func (pieces *Pieces) CheckStateAndChange(index int, previousState PieceState, newState PieceState) bool {
	pieces.mutex.Lock()

	result := false
	if pieces.pieces[index] == previousState {
		pieces.pieces[index] = newState
		result = true
	}

	pieces.mutex.Unlock()

	return result
}
