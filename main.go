package main

import (
	"flag"
	"log"
	"os"

	"github.com/mertwole/bittorent-cli/peer"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")

func main() {
	flag.Parse()

	torrentFile, err := os.Open(*torrentFileName)
	if err != nil {
		log.Fatal("Failed to open torrent file: ", err)
	}

	torrentInfo, err := torrent_info.Decode(torrentFile)
	if err != nil {
		log.Fatal("Failed to decode torrent file: ", err)
	}

	trackerResponse, err := tracker.SendRequest(torrentInfo)
	if err != nil {
		log.Fatal("Failed to send request to the tracker: ", err)
	}

	log.Printf("Discovered %d peers", len(trackerResponse.Peers))

	peer := peer.Peer{}
	err = peer.Connect(&trackerResponse.Peers[0])
	if err != nil {
		log.Fatal("Failed to connect to the peer: ", err)
	}

	err = peer.Handshake(torrentInfo)
	if err != nil {
		log.Fatal("Failed to handshake with the peer: ", err)
	}

	err = peer.StartDownload(torrentInfo)
	if err != nil {
		log.Fatal("Failed to start downloading data from peer: ", err)
	}
}
