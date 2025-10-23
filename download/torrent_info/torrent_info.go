package torrent_info

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"net/url"
	"slices"

	"github.com/mertwole/bittorrent-cli/download/bencode"
)

// TODO: Nest Metadata here.
type TorrentInfo struct {
	Trackers    []*url.URL
	Pieces      [][sha1.Size]byte
	PieceLength uint64
	TotalLength uint64
	Name        string
	Files       []FileInfo

	InfoHash [sha1.Size]byte
}

type Metadata struct {
	Pieces      [][sha1.Size]byte
	PieceLength uint64
	Name        string
	Files       []FileInfo
	TotalLength uint64

	InfoHash [sha1.Size]byte
}

type FileInfo struct {
	Path   []string
	Length uint64
}

type bencodeTorrent struct {
	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list"`
	Info         bencodeInfo `bencode:"info"`
}

type bencodeInfo struct {
	Pieces      string             `bencode:"pieces"`
	PieceLength uint64             `bencode:"piece length"`
	Name        string             `bencode:"name"`
	Files       *[]bencodeFileInfo `bencode:"files"`
	Length      *uint64            `bencode:"length"`
}

type bencodeFileInfo struct {
	Path   []string `bencode:"path"`
	Length uint64   `bencode:"length"`
}

func Decode(reader io.Reader) (*TorrentInfo, error) {
	bencodeTorrent := bencodeTorrent{}
	err := bencode.Deserialize(reader, &bencodeTorrent)
	if err != nil {
		return nil, err
	}

	trackers := make([]*url.URL, 0)

	tracker, err := url.Parse(bencodeTorrent.Announce)
	if err != nil {
		return nil, fmt.Errorf("failed to parse announce URL %s: %w", bencodeTorrent.Announce, err)
	}
	trackers = append(trackers, tracker)

	for _, list := range bencodeTorrent.AnnounceList {
		for _, tracker := range list {
			trackerURL, err := url.Parse(tracker)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to parse announce-list URL %s: %w",
					bencodeTorrent.Announce,
					err,
				)
			}

			trackers = append(trackers, trackerURL)
		}
	}

	var pieces [][sha1.Size]byte

	for chunk := range slices.Chunk([]byte(bencodeTorrent.Info.Pieces), sha1.Size) {
		if len(chunk) != sha1.Size {
			return nil, fmt.Errorf("invalid piece hash size: expected %d and got %d", sha1.Size, len(chunk))
		}

		pieces = append(pieces, [sha1.Size]byte(chunk))
	}

	totalLength := uint64(0)
	files := make([]FileInfo, 0)
	if bencodeTorrent.Info.Files != nil {
		for _, file := range *bencodeTorrent.Info.Files {
			totalLength += file.Length
			files = append(files, FileInfo(file))
		}
	} else if bencodeTorrent.Info.Length == nil {
		return nil, fmt.Errorf("cannot parse either length or file list")
	} else {
		totalLength = *bencodeTorrent.Info.Length
	}

	var serializedInfo bytes.Buffer
	err = bencode.Serialize(&serializedInfo, &bencodeTorrent.Info)
	if err != nil {
		return nil, err
	}

	info_hash := sha1.Sum(serializedInfo.Bytes())

	return &TorrentInfo{
		Trackers:    trackers,
		Pieces:      pieces,
		PieceLength: bencodeTorrent.Info.PieceLength,
		TotalLength: totalLength,
		Name:        bencodeTorrent.Info.Name,
		Files:       files,
		InfoHash:    info_hash,
	}, nil
}

func DecodeMetadata(reader io.Reader) (*Metadata, error) {
	bencodeMetadata := bencodeInfo{}
	err := bencode.Deserialize(reader, &bencodeMetadata)
	if err != nil {
		return nil, err
	}

	var pieces [][sha1.Size]byte

	for chunk := range slices.Chunk([]byte(bencodeMetadata.Pieces), sha1.Size) {
		if len(chunk) != sha1.Size {
			return nil, fmt.Errorf("invalid piece hash size: expected %d and got %d", sha1.Size, len(chunk))
		}

		pieces = append(pieces, [sha1.Size]byte(chunk))
	}

	totalLength := uint64(0)
	files := make([]FileInfo, 0)
	if bencodeMetadata.Files != nil {
		for _, file := range *bencodeMetadata.Files {
			totalLength += file.Length
			files = append(files, FileInfo(file))
		}
	} else if bencodeMetadata.Length == nil {
		return nil, fmt.Errorf("cannot parse either length or file list")
	} else {
		totalLength = *bencodeMetadata.Length
	}

	var serializedInfo bytes.Buffer
	err = bencode.Serialize(&serializedInfo, &bencodeMetadata)
	if err != nil {
		return nil, err
	}

	info_hash := sha1.Sum(serializedInfo.Bytes())

	return &Metadata{
		Pieces:      pieces,
		PieceLength: bencodeMetadata.PieceLength,
		TotalLength: totalLength,
		Name:        bencodeMetadata.Name,
		Files:       files,
		InfoHash:    info_hash,
	}, nil
}
