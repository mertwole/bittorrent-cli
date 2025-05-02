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
	InfoHash    [sha1.Size]byte
}

type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
}

type bencodeTorrent struct {
	Announce string      `bencode:"announce"`
	Info     bencodeInfo `bencode:"info"`
}

func Decode(reader io.Reader) (*TorrentInfo, error) {
	bencodeFile := bencodeTorrent{}
	err := bencode.Unmarshal(reader, &bencodeFile)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(bencodeFile.Announce)
	if err != nil {
		return nil, err
	}

	var pieces [][sha1.Size]byte

	for chunk := range slices.Chunk([]byte(bencodeFile.Info.Pieces), sha1.Size) {
		if len(chunk) != sha1.Size {
			return nil, fmt.Errorf("invalid piece hash size: expected %d and got %d", sha1.Size, len(chunk))
		}

		pieces = append(pieces, [sha1.Size]byte(chunk))
	}

	var marshalledInfo bytes.Buffer
	err = bencode.Marshal(&marshalledInfo, bencodeFile.Info)
	if err != nil {
		return nil, err
	}

	info_hash := sha1.Sum(marshalledInfo.Bytes())

	return &TorrentInfo{
		Announce:    url,
		Pieces:      pieces,
		PieceLength: bencodeFile.Info.PieceLength,
		Length:      bencodeFile.Info.Length,
		Name:        bencodeFile.Info.Name,
		InfoHash:    info_hash,
	}, nil
}
