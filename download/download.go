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

func Start(fileName string, totalLength int, pieces chan DownloadedPiece) error {
	file, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", fileName, err)
	}

	// TODO: Check if the file already contains valid pieces.

	for i := 0; i < totalLength/initialWriteChunkSize; i++ {
		file.Write(make([]byte, initialWriteChunkSize))
	}
	file.Write(make([]byte, totalLength%initialWriteChunkSize))

	go writePieces(file, pieces)

	return nil
}

func writePieces(file *os.File, pieces chan DownloadedPiece) {
	defer file.Close()

	for {
		piece := <-pieces

		_, err := file.WriteAt(piece.Data, int64(piece.Offset))
		if err != nil {
			log.Fatalf("failed to write to the download file: %v", err)
		}
	}
}
