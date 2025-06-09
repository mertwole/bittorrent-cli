package download

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
)

type DownloadedPiece struct {
	Offset int
	Data   []byte
}

type Download struct {
	files       []downloadedFile
	pieceLength int
	pieceHashes [][sha1.Size]byte
	status      Status
	mutex       sync.RWMutex
}

type downloadedFile struct {
	path   string
	length int
	handle *os.File
}

type Status struct {
	State    State
	Progress int
	Total    int
	mutex    sync.RWMutex
}

type State uint8

const (
	PreparingFiles State = 0
	CheckingHashes State = 1
	Ready          State = 2
)

func NewDownload(
	torrent *torrent_info.TorrentInfo,
	targetFolder string,
) *Download {
	downloadedFiles := Download{
		pieceLength: torrent.PieceLength,
		pieceHashes: torrent.Pieces,
		status:      Status{State: PreparingFiles, Progress: 0, Total: 0},
	}

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

func (download *Download) Prepare(pieces *pieces.Pieces) error {
	anyOpened := false
	for i, file := range download.files {
		fileHandle, fileAction, err := createOrOpenFile(file.path, file.length)
		if err != nil {
			return err
		}

		download.files[i].handle = fileHandle

		if fileAction == opened {
			anyOpened = true
		}
	}

	if anyOpened {
		download.status.mutex.Lock()
		download.status.State = CheckingHashes
		download.status.Total = pieces.Length()
		download.status.mutex.Unlock()

		err := download.scanDonePieces(pieces)
		if err != nil {
			return fmt.Errorf("failed to scan downloaded files for already downloaded pieces: %w", err)
		}
	}

	download.status.mutex.Lock()
	download.status.State = Ready
	download.status.mutex.Unlock()

	return nil
}

func (download *Download) GetStatus() Status {
	download.status.mutex.RLock()
	defer download.status.mutex.RUnlock()

	return download.status
}

func (download *Download) ReadPiece(piece int) (*[]byte, error) {
	offset := piece * download.pieceLength

	currentOffset := 0
	bytesRead := 0
	readData := make([]byte, 0)

	for _, file := range download.files {
		if file.length+currentOffset > offset {
			bytesToRead := min(download.pieceLength-bytesRead, file.length+currentOffset-offset, file.length)
			readBytes := make([]byte, bytesToRead)

			readOffset := int64(max(0, offset-currentOffset))

			download.mutex.RLock()
			_, err := file.handle.ReadAt(readBytes, readOffset)
			download.mutex.RUnlock()

			if err != nil {
				return nil, fmt.Errorf("failed to read from file %s: %w", file.path, err)
			}
			readData = append(readData, readBytes...)

			bytesRead += bytesToRead
			if bytesRead >= download.pieceLength {
				break
			}
		}

		currentOffset += file.length
	}

	return &readData, nil
}

func (download *Download) WritePiece(piece DownloadedPiece) error {
	currentOffset := 0
	bytesWritten := 0
	for _, file := range download.files {
		if file.length+currentOffset > piece.Offset {
			bytesToWrite := min(len(piece.Data)-bytesWritten, file.length+currentOffset-piece.Offset)

			writeOffset := int64(max(0, piece.Offset-currentOffset))

			download.mutex.Lock()
			_, err := file.handle.WriteAt((piece.Data)[bytesWritten:bytesWritten+bytesToWrite], writeOffset)
			download.mutex.Unlock()

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

func (download *Download) Finalize() {
	for _, file := range download.files {
		file.handle.Close()
	}
}

type createOrOpenFileAction uint8

const (
	none    createOrOpenFileAction = 0
	created createOrOpenFileAction = 1
	opened  createOrOpenFileAction = 2
)

func createOrOpenFile(path string, expectedLength int) (*os.File, createOrOpenFileAction, error) {
	var file *os.File

	fileAction := opened

	fileInfo, err := os.Stat(path)
	if err == nil && fileInfo.Size() == int64(expectedLength) {
		file, err = os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			return nil, none, fmt.Errorf("failed to open output file %s: %w", path, err)
		}
	}

	if file == nil {
		dir := filepath.Dir(path)
		err = os.MkdirAll(dir, 0770)
		if err != nil {
			return nil, none, fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}

		file, err = os.Create(path)
		if err != nil {
			return nil, none, fmt.Errorf("failed to create output file %s: %w", path, err)
		}

		err = file.Truncate(int64(expectedLength))
		if err != nil {
			return nil, none, fmt.Errorf("failed to truncate output file %s: %w", path, err)
		}

		fileAction = created
	}

	return file, fileAction, nil
}

func (download *Download) scanDonePieces(pcs *pieces.Pieces) error {
	for i, pieceHash := range download.pieceHashes {
		piece, err := download.ReadPiece(i)
		if err != nil {
			return fmt.Errorf("failed to read piece #%d: %w", i, err)
		}
		readPieceHash := sha1.Sum(*piece)

		if readPieceHash == pieceHash {
			pcs.CheckStateAndChange(i, pieces.NotDownloaded, pieces.Downloaded)
		}

		download.status.mutex.Lock()
		download.status.Progress++
		download.status.mutex.Unlock()
	}

	return nil
}
