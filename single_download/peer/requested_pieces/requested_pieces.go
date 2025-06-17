package requested_pieces

import (
	"slices"
	"sync"
)

// TODO: Upper bound on this?
type RequestedPieces struct {
	pieces []PieceRequest
	mutex  sync.RWMutex
}

type PieceRequest struct {
	Piece  int
	Offset int
	Length int
}

func (requestedPieces *RequestedPieces) AddRequest(request PieceRequest) {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	if slices.Contains(requestedPieces.pieces, request) {
		return
	}

	requestedPieces.pieces = append(requestedPieces.pieces, request)
}

func (requestedPieces *RequestedPieces) CancelRequest(request PieceRequest) {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	idx := slices.Index(requestedPieces.pieces, request)
	if idx != -1 {
		requestedPieces.pieces = slices.Delete(requestedPieces.pieces, idx, idx+1)
	}
}

func (requestedPieces *RequestedPieces) PopRequest() *PieceRequest {
	requestedPieces.mutex.Lock()
	defer requestedPieces.mutex.Unlock()

	if len(requestedPieces.pieces) == 0 {
		return nil
	}

	request := requestedPieces.pieces[0]
	requestedPieces.pieces = requestedPieces.pieces[1:]

	return &request
}
