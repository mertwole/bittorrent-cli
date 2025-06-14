package single_download

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
)

const discoveredPeersQueueSize = 16

type Download struct {
	Pieces           *pieces.Pieces
	DownloadedPieces *download.Download
	torrentInfo      *torrent_info.TorrentInfo

	peerCount atomic.Int32
}

func New(fileName string, downloadFolderName string) (*Download, error) {
	torrentFile, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %w", err)
	}

	torrentInfo, err := torrent_info.Decode(torrentFile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode torrent file: %w", err)
	}

	pieces := pieces.New(len(torrentInfo.Pieces))
	downloadedPieces := download.NewDownload(torrentInfo, downloadFolderName)

	return &Download{Pieces: pieces, DownloadedPieces: downloadedPieces, torrentInfo: torrentInfo}, nil
}

func (download *Download) Start() {
	err := download.DownloadedPieces.Prepare(download.Pieces)
	if err != nil {
		log.Fatalf("failed to prepare download files: %v", err)
	}

	piecesBitfield := download.Pieces.GetBitfield()
	downloadedCount := (&piecesBitfield).SetPiecesCount()
	log.Printf("Discovered %d already downloaded pieces", downloadedCount)

	peerID := [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	discoveredPeers := make(chan tracker.PeerInfo, discoveredPeersQueueSize)

	for _, trackerURL := range download.torrentInfo.Trackers {
		tracker := tracker.NewTracker(trackerURL,
			download.torrentInfo.InfoHash,
			download.torrentInfo.TotalLength,
			peerID,
		)
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
			go download.downloadFromPeer(&newPeer)
		}
	}
}

func (download *Download) GetPeerCount() int {
	return int(download.peerCount.Load())
}

func (download *Download) GetTorrentName() string {
	return download.torrentInfo.Name
}

func (download *Download) downloadFromPeer(peerInfo *tracker.PeerInfo) {
	for {
		peer := peer.Peer{}
		err := peer.Connect(peerInfo)
		if err != nil {
			log.Printf("failed to connect to the peer: %v", err)
			return
		}

		err = peer.Handshake(download.torrentInfo)
		if err != nil {
			log.Printf("failed to handshake with the peer: %v", err)
			return
		}

		log.Printf("connected to the peer %+v", peerInfo)

		download.peerCount.Add(1)

		err = peer.StartExchange(download.torrentInfo, download.Pieces, download.DownloadedPieces)
		if err != nil {
			download.peerCount.Add(-1)
			log.Printf("failed to download data from peer: %v. reconnecting", err)
		}
	}
}
