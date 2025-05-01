package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/jackpal/bencode-go"
)

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")

func main() {
	flag.Parse()

	torrentFile, err := os.Open(*torrentFileName)
	if err != nil {
		log.Fatal("Failed to open torrent file: ", err)
	}

	bencodeTorrent, err := DecodeTorrentFile(torrentFile)
	if err != nil {
		log.Fatal("Failed to decode torrent file: ", err)
	}

	var _ = bencodeTorrent
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

func DecodeTorrentFile(r io.Reader) (*bencodeTorrent, error) {
	bto := bencodeTorrent{}
	err := bencode.Unmarshal(r, &bto)
	if err != nil {
		return nil, err
	}
	return &bto, nil
}
