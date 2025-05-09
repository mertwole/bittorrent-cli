package piece_scheduler

import (
	"slices"
)

const channelCapacity = 4096

type PieceScheduler struct {
	remainingPieces []int
}

func Create(totalPieces int, donePieces []int) PieceScheduler {
	scheduler := PieceScheduler{remainingPieces: make([]int, 0)}

	for piece := range totalPieces {
		if slices.Contains(donePieces, piece) {
			continue
		}

		scheduler.remainingPieces = append(scheduler.remainingPieces, piece)
	}

	return scheduler
}

func (scheduler *PieceScheduler) Start() chan int {
	pieceChannel := make(chan int, channelCapacity)

	go scheduler.sendPieces(pieceChannel)

	return pieceChannel
}

func (scheduler *PieceScheduler) sendPieces(pieceChannel chan<- int) {
	for _, piece := range scheduler.remainingPieces {
		pieceChannel <- piece
	}
}
