package single_download

import (
	"fmt"
	"log"
	"os"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
)

const discoveredPeersQueueSize = 16

func StartDownload(fileName string, downloadFolderName string) (*pieces.Pieces, *download.Download, error) {
	torrentFile, err := os.Open(fileName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open torrent file: %w", err)
	}

	torrentInfo, err := torrent_info.Decode(torrentFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode torrent file: %w", err)
	}

	pieces := pieces.New(len(torrentInfo.Pieces))
	downloadedPieces := download.NewDownload(torrentInfo, downloadFolderName)

	go startDownloadInner(torrentInfo, pieces, downloadedPieces)

	return pieces, downloadedPieces, nil
}

func startDownloadInner(
	torrentInfo *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
	downloadedPieces *download.Download,
) {
	err := downloadedPieces.Prepare(pieces)
	if err != nil {
		log.Fatalf("failed to prepare download files: %v", err)
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
			log.Printf("failed to connect to the peer: %v", err)
			return
		}

		err = peer.Handshake(torrentInfo)
		if err != nil {
			log.Printf("failed to handshake with the peer: %v", err)
			return
		}

		log.Printf("connected to the peer %+v", peerInfo)

		err = peer.StartExchange(torrentInfo, pieces, downloadedPieces)
		if err != nil {
			log.Printf("failed to download data from peer: %v. reconnecting", err)
		}
	}
}
