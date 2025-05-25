package download

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mertwole/bittorrent-cli/torrent_info"
)

const initialWriteChunkSize = 1024

type DownloadedPiece struct {
	Offset int
	Data   []byte
}

type Download struct {
	files       []downloadedFile
	pieceLength int
	pieceHashes [][sha1.Size]byte
	mutex       sync.RWMutex
}

type downloadedFile struct {
	path   string
	length int
	handle *os.File
}

type DownloadStatus struct {
	DonePieces []int
}

func NewDownload(torrent *torrent_info.TorrentInfo, targetFolder string) (*Download, *DownloadStatus, error) {
	download := newDownloadInner(torrent, targetFolder)

	err := download.createOrOpenAll()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create or open downloaded files: %w", err)
	}

	donePieces, err := download.scanDonePieces()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to scan downloaded files for already downloaded pieces: %w", err)
	}

	status := &DownloadStatus{DonePieces: donePieces}

	return download, status, nil
}

func (files *Download) ReadPiece(piece int) (*[]byte, error) {
	offset := piece * files.pieceLength

	currentOffset := 0
	bytesRead := 0
	readData := make([]byte, 0)

	for _, file := range files.files {
		if file.length+currentOffset > offset {
			bytesToRead := min(files.pieceLength-bytesRead, file.length+currentOffset-offset, file.length)
			readBytes := make([]byte, bytesToRead)

			readOffset := int64(max(0, offset-currentOffset))

			files.mutex.RLock()
			_, err := file.handle.ReadAt(readBytes, readOffset)
			files.mutex.RUnlock()

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

func (files *Download) WritePiece(piece DownloadedPiece) error {
	currentOffset := 0
	bytesWritten := 0
	for _, file := range files.files {
		if file.length+currentOffset > piece.Offset {
			bytesToWrite := min(len(piece.Data)-bytesWritten, file.length+currentOffset-piece.Offset)

			writeOffset := int64(max(0, piece.Offset-currentOffset))

			files.mutex.Lock()
			_, err := file.handle.WriteAt((piece.Data)[bytesWritten:bytesWritten+bytesToWrite], writeOffset)
			files.mutex.Unlock()

			if err != nil {
				return fmt.Errorf("failed to write to file %s: %w", file.path, err)
			}

			bytesWritten += bytesToWrite
			if bytesWritten >= len(piece.Data) {
				break
			}
		}

		currentOffset += file.length
	}

	return nil
}

func (files *Download) Finalize() {
	for _, file := range files.files {
		file.handle.Close()
	}
}

func newDownloadInner(torrent *torrent_info.TorrentInfo, targetFolder string) *Download {
	downloadedFiles := Download{pieceLength: torrent.PieceLength, pieceHashes: torrent.Pieces}

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

func (files *Download) createOrOpenAll() error {
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

func (files *Download) scanDonePieces() ([]int, error) {
	donePieces := make([]int, 0)

	for i, pieceHash := range files.pieceHashes {
		piece, err := files.ReadPiece(i)
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
