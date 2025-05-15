package pieces

import (
	"sync"

	"github.com/mertwole/bittorrent-cli/bitfield"
)

type Pieces struct {
	mutex      sync.RWMutex
	pieces     []PieceState
	downloaded bitfield.Bitfield
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

	bitfield := bitfield.NewEmptyBitfield(count)

	for _, downloaded := range *downloaded {
		pieces[downloaded] = Downloaded
		bitfield.AddPiece(downloaded)
	}

	return &Pieces{pieces: pieces, downloaded: bitfield}
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

	return pieces.downloaded
}

func (pieces *Pieces) CheckStateAndChange(index int, previousState PieceState, newState PieceState) bool {
	pieces.mutex.Lock()
	defer pieces.mutex.Unlock()

	result := false
	if pieces.pieces[index] == previousState {
		pieces.pieces[index] = newState

		if newState == Downloaded {
			pieces.downloaded.AddPiece(index)
		} else {
			pieces.downloaded.RemovePiece(index)
		}

		result = true
	}

	return result
}
