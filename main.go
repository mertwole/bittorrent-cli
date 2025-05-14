package main

import (
	"flag"
	"log"
	"os"

	"github.com/mertwole/bittorent-cli/download"
	"github.com/mertwole/bittorent-cli/peer"
	"github.com/mertwole/bittorent-cli/pieces"
	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

const discoveredPeersQueueSize = 16

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")
var downloadFolderName = flag.String("download", "./data", "Path to the download folder")

func main() {
	flag.Parse()

	logFile, err := os.OpenFile("log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatalf("error opening log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)

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

	pieces := pieces.NewPieces(len(torrentInfo.Pieces), &donePieces)

	peerID := [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	discoveredPeers := make(chan tracker.PeerInfo, discoveredPeersQueueSize)

	for _, trackerURL := range torrentInfo.Trackers {
		tracker := tracker.NewTracker(trackerURL, torrentInfo.InfoHash, torrentInfo.TotalLength, peerID)
		go tracker.ListenForPeers(discoveredPeers)
	}

	knownPeers := make([]tracker.PeerInfo, 0)
	for {
		newPeer := <-discoveredPeers
		alreadyKnown := false
		for _, peer := range knownPeers {
			if peer.IP.Equal(newPeer.IP) {
				alreadyKnown = true
				break
			}
		}

		if !alreadyKnown {
			knownPeers = append(knownPeers, newPeer)
			go downloadFromPeer(&newPeer, torrentInfo, pieces, downloadedPiecesChannel)
		}
	}
}

func downloadFromPeer(
	peerInfo *tracker.PeerInfo,
	torrentInfo *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
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

	err = peer.StartDownload(torrentInfo, pieces, downloadedPiecesChannel)
	if err != nil {
		log.Fatal("Failed to start downloading data from peer: ", err)
	}
}
