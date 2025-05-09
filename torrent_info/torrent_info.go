package torrent_info

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"net/url"
	"slices"

	"github.com/jackpal/bencode-go"
)

type TorrentInfo struct {
	Trackers    []*url.URL
	Pieces      [][sha1.Size]byte
	PieceLength int
	Length      int
	Name        string
	Files       []FileInfo
	InfoHash    [sha1.Size]byte
}

type FileInfo struct {
	Path   []string
	Length int
}

type bencodeInfo struct {
	Pieces      string            `bencode:"pieces"`
	PieceLength int               `bencode:"piece length"`
	Length      int               `bencode:"length"`
	Name        string            `bencode:"name"`
	Files       []bencodeFileInfo `bencode:"files"`
}

type bencodeInfoSingleFile struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeInfoMultiFile struct {
	Pieces      string            `bencode:"pieces"`
	PieceLength int               `bencode:"piece length"`
	Name        string            `bencode:"name"`
	Files       []bencodeFileInfo `bencode:"files"`
}

type bencodeFileInfo struct {
	Path   []string `bencode:"path"`
	Length int      `bencode:"length"`
}

type bencodeTorrent struct {
	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list"`
	Info         bencodeInfo `bencode:"info"`
}

func Decode(reader io.Reader) (*TorrentInfo, error) {
	bencodeTorrent := bencodeTorrent{}
	err := bencode.Unmarshal(reader, &bencodeTorrent)
	if err != nil {
		return nil, err
	}

	trackers := make([]*url.URL, 0)

	tracker, err := url.Parse(bencodeTorrent.Announce)
	if err != nil {
		return nil, fmt.Errorf("failed to parse announce URL %s: %w", bencodeTorrent.Announce, err)
	}
	trackers = append(trackers, tracker)

	if bencodeTorrent.AnnounceList != nil {
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
	}

	var pieces [][sha1.Size]byte

	for chunk := range slices.Chunk([]byte(bencodeTorrent.Info.Pieces), sha1.Size) {
		if len(chunk) != sha1.Size {
			return nil, fmt.Errorf("invalid piece hash size: expected %d and got %d", sha1.Size, len(chunk))
		}

		pieces = append(pieces, [sha1.Size]byte(chunk))
	}

	files := make([]FileInfo, 0)
	for _, bencodeFile := range bencodeTorrent.Info.Files {
		files = append(files, FileInfo{Path: bencodeFile.Path, Length: bencodeFile.Length})
	}

	var marshalledInfo bytes.Buffer
	if len(files) == 0 {
		info := bencodeInfoSingleFile{
			Pieces:      bencodeTorrent.Info.Pieces,
			PieceLength: bencodeTorrent.Info.PieceLength,
			Length:      bencodeTorrent.Info.Length,
			Name:        bencodeTorrent.Info.Name,
		}

		err = bencode.Marshal(&marshalledInfo, info)
		if err != nil {
			return nil, err
		}
	} else {
		info := bencodeInfoMultiFile{
			Pieces:      bencodeTorrent.Info.Pieces,
			PieceLength: bencodeTorrent.Info.PieceLength,
			Name:        bencodeTorrent.Info.Name,
			Files:       bencodeTorrent.Info.Files,
		}

		err = bencode.Marshal(&marshalledInfo, info)
		if err != nil {
			return nil, err
		}
	}

	info_hash := sha1.Sum(marshalledInfo.Bytes())

	return &TorrentInfo{
		Trackers:    trackers,
		Pieces:      pieces,
		PieceLength: bencodeTorrent.Info.PieceLength,
		Length:      bencodeTorrent.Info.Length,
		Name:        bencodeTorrent.Info.Name,
		Files:       files,
		InfoHash:    info_hash,
	}, nil
}
