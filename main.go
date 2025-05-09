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

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")
var downloadFolderName = flag.String("download", "./data", "Path to the download folder")

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

	peersInfo := make([]tracker.PeerInfo, 0)
	for _, trackerURL := range torrentInfo.Trackers {
		trackerResponse, err := tracker.SendRequest(trackerURL, torrentInfo.InfoHash, torrentInfo.TotalLength)
		if err != nil {
			log.Printf("Failed to send request to the tracker: %v", err)
			continue
		}

		log.Printf("Discovered %d peers: %v", len(trackerResponse.Peers), trackerResponse.Peers)

		peersInfo = append(peersInfo, trackerResponse.Peers[:]...)
	}

	for _, peerInfo := range peersInfo {
		go downloadFromPeer(&peerInfo, torrentInfo, requestedPiecesChannel, downloadedPiecesChannel)
	}

	for {
		time.Sleep(time.Millisecond * 100)
	}
}

func downloadFromPeer(
	peerInfo *tracker.PeerInfo,
	torrentInfo *torrent_info.TorrentInfo,
	requestedPiecesChannel chan int,
	downloadedPiecesChannel chan<- download.DownloadedPiece,
) {
	peer := peer.Peer{}
	err := peer.Connect(peerInfo)
	if err != nil {
		log.Printf("Failed to connect to the peer: %v", err)
		return
	}

	err = peer.Handshake(torrentInfo)
	if err != nil {
		log.Printf("Failed to handshake with the peer: %v", err)
		return
	}

	log.Printf("connected to the peer %+v", peerInfo)

	err = peer.StartDownload(torrentInfo, requestedPiecesChannel, downloadedPiecesChannel)
	if err != nil {
		log.Fatal("Failed to start downloading data from peer: ", err)
	}
}
