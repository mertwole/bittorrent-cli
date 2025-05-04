package download

import (
	"fmt"
	"log"
	"os"
)

type DownloadedPiece struct {
	Offset int
	Data   []byte
}

const initialWriteChunkSize = 1024

func Start(fileName string, totalLength int) (chan<- DownloadedPiece, error) {
	file, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file %s: %w", fileName, err)
	}

	// TODO: Check if the file already contains valid pieces.

	for range totalLength / initialWriteChunkSize {
		file.Write(make([]byte, initialWriteChunkSize))
	}
	file.Write(make([]byte, totalLength%initialWriteChunkSize))

	pieces := make(chan DownloadedPiece)

	go writePieces(file, pieces)

	return pieces, nil
}

func writePieces(file *os.File, pieces <-chan DownloadedPiece) {
	defer file.Close()

	for {
		piece := <-pieces

		_, err := file.WriteAt(piece.Data, int64(piece.Offset))
		if err != nil {
			log.Fatalf("failed to write to the download file: %v", err)
		}
	}
}
