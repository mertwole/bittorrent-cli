package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/mertwole/bittorent-cli/download"
	"github.com/mertwole/bittorent-cli/peer"
	"github.com/mertwole/bittorent-cli/piece_scheduler"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

const maxConnections = 8

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")
var downloadFolderName = flag.String("download", "./data", "Path to the downloaded folder")

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

	downloadedPiecesChannel, donePieces, err := download.Start(torrentInfo, *downloadFolderName)
	if err != nil {
		log.Fatal("Failed to start download service: ", err)
	}

	log.Printf("Discovered %d already downloaded pieces", len(donePieces))

	pieceScheduler := piece_scheduler.Create(len(torrentInfo.Pieces), donePieces)
	requestedPiecesChannel := pieceScheduler.Start()

	trackerResponse, err := tracker.SendRequest(torrentInfo)
	if err != nil {
		log.Fatal("Failed to send request to the tracker: ", err)
	}

	log.Printf("Discovered %d peers", len(trackerResponse.Peers))

	if len(trackerResponse.Peers) == 0 {
		return
	}

	peers := make([]peer.Peer, 0)

	for _, peerInfo := range trackerResponse.Peers {
		peer := peer.Peer{}
		err = peer.Connect(&peerInfo)
		if err != nil {
			log.Printf("Failed to connect to the peer: %v", err)
			continue
		}

		err = peer.Handshake(torrentInfo)
		if err != nil {
			log.Printf("Failed to handshake with the peer: %v", err)
			continue
		}

		log.Printf("connected to the peer %+v", peerInfo)

		err = peer.StartDownload(torrentInfo, requestedPiecesChannel, downloadedPiecesChannel)
		if err != nil {
			log.Fatal("Failed to start downloading data from peer: ", err)
		}

		peers = append(peers, peer)

		if len(peers) >= maxConnections {
			break
		}
	}

	for {
		time.Sleep(time.Millisecond * 100)
	}
}
