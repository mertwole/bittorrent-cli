package piece_scheduler

import (
	"slices"
)

const channelCapacity = 128

type PieceScheduler struct {
	remainingPieces []int
}

func Create(totalPieces int, donePieces []int) PieceScheduler {
	scheduler := PieceScheduler{remainingPieces: make([]int, 0)}

	for piece := range totalPieces {
		if slices.Contains(donePieces, piece) {
			continue
		}

		scheduler.remainingPieces = append([]int{piece}, scheduler.remainingPieces...)
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
