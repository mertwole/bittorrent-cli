package download

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mertwole/bittorent-cli/torrent_info"
)

type DownloadedPiece struct {
	Offset int
	Data   []byte
}

const initialWriteChunkSize = 1024

func Start(torrent *torrent_info.TorrentInfo, targetFolder string) (chan<- DownloadedPiece, []int, error) {
	fullPath := filepath.Join(targetFolder, torrent.Name)

	donePieces := make([]int, 0)

	var file *os.File

	fileInfo, err := os.Stat(fullPath)
	if err == nil && fileInfo.Size() == int64(torrent.Length) {
		file, err = os.OpenFile(fullPath, os.O_RDWR, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open output file %s: %w", fullPath, err)
		}

		donePieces, err = scanDonePieces(file, torrent)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan downloaded pieces from the file %s: %w", fullPath, err)
		}
	}

	if file == nil {
		file, err = os.Create(fullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create output file %s: %w", fullPath, err)
		}

		for range torrent.Length / initialWriteChunkSize {
			file.Write(make([]byte, initialWriteChunkSize))
		}
		file.Write(make([]byte, torrent.Length%initialWriteChunkSize))
	}

	pieces := make(chan DownloadedPiece)

	go writePieces(file, pieces)

	return pieces, donePieces, nil
}

func scanDonePieces(file *os.File, torrent *torrent_info.TorrentInfo) ([]int, error) {
	donePieces := make([]int, 0)

	for piece, checksum := range torrent.Pieces {
		offset := piece * torrent.PieceLength
		length := min(torrent.PieceLength, torrent.Length-offset)

		writtenPiece := make([]byte, length)
		_, err := file.ReadAt(writtenPiece, int64(offset))
		if err != nil {
			return nil, err
		}

		writtenPieceChecksum := sha1.Sum(writtenPiece)

		if writtenPieceChecksum == checksum {
			donePieces = append(donePieces, piece)
		}
	}

	return donePieces, nil
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
