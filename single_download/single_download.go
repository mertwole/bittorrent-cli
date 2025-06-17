package single_download

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/mertwole/bittorrent-cli/global_params"
	"github.com/mertwole/bittorrent-cli/single_download/download"
	"github.com/mertwole/bittorrent-cli/single_download/lsd"
	"github.com/mertwole/bittorrent-cli/single_download/peer"
	"github.com/mertwole/bittorrent-cli/single_download/pieces"
	"github.com/mertwole/bittorrent-cli/single_download/torrent_info"
	"github.com/mertwole/bittorrent-cli/single_download/tracker"
)

const discoveredPeersQueueSize = 16
const connectedPeersQueueSize = 16

type Download struct {
	Pieces           *pieces.Pieces
	DownloadedPieces *download.Download
	torrentInfo      *torrent_info.TorrentInfo
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

	lsdErrors := make(chan error)
	go lsd.StartDiscovery(download.torrentInfo.InfoHash, discoveredPeers, lsdErrors)

	connectedPeers := make(chan connectedPeer, connectedPeersQueueSize)
	go download.acceptConnectionRequests(connectedPeers)

	go download.downloadFromAllPeers(discoveredPeers, connectedPeers)

	// TODO: Process errors.
	err = <-lsdErrors
	log.Printf("error in lsd: %v", err)

	for {
		time.Sleep(time.Millisecond * 100)
	}
}

func (download *Download) GetTorrentName() string {
	return download.torrentInfo.Name
}

func (download *Download) downloadFromAllPeers(
	discoveredPeers <-chan tracker.PeerInfo,
	connectedPeers <-chan connectedPeer,
) {
	knownPeers := make([]tracker.PeerInfo, 0)
	for {
		select {
		case newPeer := <-discoveredPeers:
			alreadyKnown := false
			for _, peer := range knownPeers {
				if peer.IP.Equal(newPeer.IP) {
					alreadyKnown = true
					break
				}
			}

			if !alreadyKnown {
				knownPeers = append(knownPeers, newPeer)
				go download.downloadFromPeer(&newPeer, nil)
			}
		case newPeer := <-connectedPeers:
			alreadyKnown := false
			for _, peer := range knownPeers {
				if peer.IP.Equal(newPeer.info.IP) {
					alreadyKnown = true
					break
				}
			}

			if !alreadyKnown {
				knownPeers = append(knownPeers, newPeer.info)
				go download.downloadFromPeer(&newPeer.info, newPeer.connection)
			}
		}
	}
}

func (download *Download) downloadFromPeer(peerInfo *tracker.PeerInfo, connection *net.Conn) {
	for {
		peer := peer.Peer{}
		err := peer.Connect(peerInfo, connection)
		if err != nil {
			log.Printf("failed to connect to the peer: %v", err)
			return
		}

		err = peer.Handshake(download.torrentInfo)
		if err != nil {
			log.Printf("failed to handshake with the peer: %v", err)
			return
		}

		log.Printf("handshaked with the peer %+v", peerInfo)

		err = peer.StartExchange(download.torrentInfo, download.Pieces, download.DownloadedPieces)
		if err != nil {
			log.Printf("failed to download data from peer: %v. reconnecting", err)
		}
	}
}

type connectedPeer struct {
	info       tracker.PeerInfo
	connection *net.Conn
}

func (download *Download) acceptConnectionRequests(connectedPeers chan<- connectedPeer) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", global_params.ConnectionListenPort))
	if err != nil {
		log.Fatalf("failed to create TCP listener")
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept TCP connection: %v", err)
			continue
		}

		remoteAddress := conn.RemoteAddr().String()
		remoteAddrPort, err := netip.ParseAddrPort(remoteAddress)
		if err != nil {
			log.Panicf("unable to parse address and port: %v", err)
		}
		remoteIP := remoteAddrPort.Addr().As4()

		peerInfo := tracker.PeerInfo{IP: remoteIP[:], Port: remoteAddrPort.Port()}

		log.Printf("accepted TCP connection from %+v", peerInfo)

		connectedPeers <- connectedPeer{info: peerInfo, connection: &conn}
	}
}
