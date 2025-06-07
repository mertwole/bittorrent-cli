package main

import (
	"flag"
	"log"
	"os"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
	"github.com/mertwole/bittorrent-cli/ui"
)

const discoveredPeersQueueSize = 16

const logFileName = "log"

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")
var downloadFolderName = flag.String("download", "./data", "Path to the download folder")
var interactiveMode = flag.Bool("interactive", true, "Whether the client should be run in an interactive mode")

func main() {
	flag.Parse()

	if *interactiveMode {
		logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			log.Fatalf("error opening log file: %v", err)
		}
		defer logFile.Close()

		log.SetOutput(logFile)
	}

	torrentFile, err := os.Open(*torrentFileName)
	if err != nil {
		log.Fatal("Failed to open torrent file: ", err)
	}

	torrentInfo, err := torrent_info.Decode(torrentFile)
	if err != nil {
		log.Fatal("Failed to decode torrent file: ", err)
	}

	pieces := pieces.New(len(torrentInfo.Pieces))

	if *interactiveMode {
		go ui.StartUI(pieces)
	}

	downloadedPieces, err := download.NewDownload(torrentInfo, pieces, *downloadFolderName)
	if err != nil {
		log.Fatal("Failed to start download service: ", err)
	}

	piecesBitfield := pieces.GetBitfield()
	downloadedCount := (&piecesBitfield).SetPiecesCount()
	log.Printf("Discovered %d already downloaded pieces", downloadedCount)

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
			go downloadFromPeer(&newPeer, torrentInfo, pieces, downloadedPieces)
		}
	}
}

func downloadFromPeer(
	peerInfo *tracker.PeerInfo,
	torrentInfo *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
	downloadedPieces *download.Download,
) {
	for {
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

		err = peer.StartExchange(torrentInfo, pieces, downloadedPieces)
		if err != nil {
			log.Printf("Failed to download data from peer: %v. reconnecting", err)
		}
	}
}
