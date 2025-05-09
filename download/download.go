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
	downloadedFiles := newDownloadedFiles(torrent, targetFolder)

	err := downloadedFiles.createOrOpenAll()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create or open downloaded files: %w", err)
	}

	donePieces, err := downloadedFiles.scanDonePieces()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to scan downloaded files for already downloaded pieces: %w", err)
	}

	pieces := make(chan DownloadedPiece)

	go writePieces(downloadedFiles, pieces)

	return pieces, donePieces, nil
}

type downloadedFiles struct {
	files       []downloadedFile
	pieceLength int
	pieceHashes [][sha1.Size]byte
}

type downloadedFile struct {
	path   string
	length int
	handle *os.File
}

func newDownloadedFiles(torrent *torrent_info.TorrentInfo, targetFolder string) *downloadedFiles {
	downloadedFiles := downloadedFiles{pieceLength: torrent.PieceLength, pieceHashes: torrent.Pieces}

	if len(torrent.Files) == 0 {
		path := filepath.Join(targetFolder, torrent.Name)
		downloadedFiles.files = []downloadedFile{{path: path, length: torrent.TotalLength}}

		return &downloadedFiles
	}

	downloadFolderPath := filepath.Join(targetFolder, torrent.Name)
	downloadedFiles.files = make([]downloadedFile, len(torrent.Files))
	for i, fileInfo := range torrent.Files {
		relativePath := filepath.Join(fileInfo.Path...)
		path := filepath.Join(downloadFolderPath, relativePath)

		downloadedFiles.files[i] = downloadedFile{path: path, length: fileInfo.Length}
	}

	return &downloadedFiles
}

func (files *downloadedFiles) createOrOpenAll() error {
	for i, file := range files.files {
		fileHandle, err := createOrOpenFile(file.path, file.length)
		if err != nil {
			return err
		}

		files.files[i].handle = fileHandle
	}

	return nil
}

func createOrOpenFile(path string, expectedLength int) (*os.File, error) {
	var file *os.File

	fileInfo, err := os.Stat(path)
	if err == nil && fileInfo.Size() == int64(expectedLength) {
		file, err = os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open output file %s: %w", path, err)
		}
	}

	if file == nil {
		dir := filepath.Dir(path)
		err = os.MkdirAll(dir, 0770)
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}

		file, err = os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file %s: %w", path, err)
		}

		for range expectedLength / initialWriteChunkSize {
			file.Write(make([]byte, initialWriteChunkSize))
		}
		file.Write(make([]byte, expectedLength%initialWriteChunkSize))
	}

	return file, nil
}

func (files *downloadedFiles) scanDonePieces() ([]int, error) {
	donePieces := make([]int, 0)

	for i, pieceHash := range files.pieceHashes {
		piece, err := files.readPiece(i)
		if err != nil {
			return nil, fmt.Errorf("failed to read piece #%d: %w", i, err)
		}
		readPieceHash := sha1.Sum(*piece)

		if readPieceHash == pieceHash {
			donePieces = append(donePieces, i)
		}
	}

	return donePieces, nil
}

func (files *downloadedFiles) readPiece(piece int) (*[]byte, error) {
	offset := piece * files.pieceLength

	currentOffset := 0
	bytesRead := 0
	readData := make([]byte, 0)

	for _, file := range files.files {
		if file.length+currentOffset > offset {
			bytesToRead := min(files.pieceLength-bytesRead, file.length+currentOffset-offset)
			readBytes := make([]byte, bytesToRead)

			readOffset := int64(max(0, offset-currentOffset))

			_, err := file.handle.ReadAt(readBytes, readOffset)
			if err != nil {
				return nil, fmt.Errorf("failed to read from file %s: %w", file.path, err)
			}
			readData = append(readData, readBytes...)

			bytesRead += bytesToRead
			if bytesRead >= files.pieceLength {
				break
			}
		}

		currentOffset += file.length
	}

	return &readData, nil
}

func (files *downloadedFiles) writePiece(offset int, data *[]byte) error {
	currentOffset := 0
	bytesWritten := 0
	for _, file := range files.files {
		if file.length+currentOffset > offset {
			bytesToWrite := min(len(*data)-bytesWritten, file.length+currentOffset-offset)

			writeOffset := int64(max(0, offset-currentOffset))

			_, err := file.handle.WriteAt((*data)[bytesWritten:bytesWritten+bytesToWrite], writeOffset)
			if err != nil {
				return fmt.Errorf("failed to write to file %s: %w", file.path, err)
			}

			bytesWritten += bytesToWrite
			if bytesWritten >= len(*data) {
				break
			}
		}

		currentOffset += file.length
	}

	return nil
}

func (files *downloadedFiles) closeAll() {
	for _, file := range files.files {
		file.handle.Close()
	}
}

func writePieces(files *downloadedFiles, pieces <-chan DownloadedPiece) {
	defer files.closeAll()

	for {
		piece := <-pieces

		err := files.writePiece(piece.Offset, &piece.Data)
		if err != nil {
			log.Fatalf("failed to write to the download file: %v", err)
		}
	}
}
