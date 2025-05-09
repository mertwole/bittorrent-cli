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
	Announce    *url.URL
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

type bencodeFileInfo struct {
	Path   []string `bencode:"path"`
	Length int      `bencode:"length"`
}

type bencodeInfo struct {
	Pieces      string            `bencode:"pieces"`
	PieceLength int               `bencode:"piece length"`
	Length      int               `bencode:"length"`
	Name        string            `bencode:"name"`
	Files       []bencodeFileInfo `bencode:"files"`
}

type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

func Decode(reader io.Reader) (*TorrentInfo, error) {
	bencodeTorrent := bencodeTorrent{}
	err := bencode.Unmarshal(reader, &bencodeTorrent)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(bencodeTorrent.Announce)
	if err != nil {
		return nil, err
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
	err = bencode.Marshal(&marshalledInfo, bencodeTorrent.Info)
	if err != nil {
		return nil, err
	}

	info_hash := sha1.Sum(marshalledInfo.Bytes())

	return &TorrentInfo{
		Announce:    url,
		Pieces:      pieces,
		PieceLength: bencodeTorrent.Info.PieceLength,
		Length:      bencodeTorrent.Info.Length,
		Name:        bencodeTorrent.Info.Name,
		Files:       files,
		InfoHash:    info_hash,
	}, nil
}
