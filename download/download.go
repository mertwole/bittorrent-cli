package download

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/mertwole/bittorrent-cli/download/bitfield"
	"github.com/mertwole/bittorrent-cli/download/downloaded_files"
	"github.com/mertwole/bittorrent-cli/download/lsd"
	"github.com/mertwole/bittorrent-cli/download/peer"
	"github.com/mertwole/bittorrent-cli/download/pieces"
	"github.com/mertwole/bittorrent-cli/download/torrent_info"
	"github.com/mertwole/bittorrent-cli/download/tracker"
	"github.com/mertwole/bittorrent-cli/global_params"
)

const discoveredPeersQueueSize = 16
const connectedPeersQueueSize = 16
const setPausedChannelSize = 8
const listenRetries = 16

type Status uint8

const (
	PreparingFiles Status = iota
	CheckingHashes
	Downloading
	Paused
)

type Download struct {
	Pieces           *pieces.Pieces
	downloadedPieces *downloaded_files.DownloadedFiles
	torrentInfo      *torrent_info.TorrentInfo

	paused    bool
	setPaused chan bool

	cancelCallback context.CancelFunc
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
	downloadedPieces := downloaded_files.New(torrentInfo, downloadFolderName)

	return &Download{
		Pieces:           pieces,
		downloadedPieces: downloadedPieces,
		torrentInfo:      torrentInfo,
		setPaused:        make(chan bool, setPausedChannelSize),
	}, nil
}

func (download *Download) Start() {
	err := download.downloadedPieces.Prepare(download.Pieces)
	if err != nil {
		log.Fatalf("failed to prepare download files: %v", err)
	}

	piecesBitfield := download.Pieces.GetBitfield()
	downloadedCount := (&piecesBitfield).SetPiecesCount()
	log.Printf("Discovered %d already downloaded pieces", downloadedCount)

	peerID := [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	discoveredPeers := make(chan tracker.PeerInfo, discoveredPeersQueueSize)

	ctx, cancel := context.WithCancel(context.Background())
	download.cancelCallback = cancel

	connectedPeers := make(chan connectedPeer, connectedPeersQueueSize)
	listener, listenPort, err := createTCPListener()
	if err != nil {
		log.Printf("failed to create TCP listener: %v", err)
	} else {
		go download.acceptConnectionRequests(ctx, listener, connectedPeers)
	}

	for _, trackerURL := range download.torrentInfo.Trackers {
		tracker := tracker.NewTracker(trackerURL,
			download.torrentInfo.InfoHash,
			download.torrentInfo.TotalLength,
			peerID,
		)
		go tracker.ListenForPeers(ctx, listenPort, discoveredPeers)
	}

	// TODO: It should be shared across all the downloads.
	lsdErrors := make(chan error)
	go lsd.StartDiscovery(download.torrentInfo.InfoHash, discoveredPeers, listenPort, lsdErrors)

	go download.downloadFromAllPeers(discoveredPeers, connectedPeers)

	// TODO: Process errors.
	err = <-lsdErrors
	log.Printf("error in lsd: %v", err)

	for {
		time.Sleep(time.Millisecond * 100)
	}
}

func (download *Download) Stop() {
	download.setPaused <- true

	if download.cancelCallback != nil {
		download.cancelCallback()
	}
}

func (download *Download) TogglePause() {
	download.paused = !download.paused
	download.setPaused <- download.paused
}

func (download *Download) GetTorrentName() string {
	return download.torrentInfo.Name
}

func (download *Download) GetStatus() Status {
	if download.paused {
		return Paused
	}

	downloadState := download.downloadedPieces.GetStatus().State
	switch downloadState {
	case downloaded_files.PreparingFiles:
		return PreparingFiles
	case downloaded_files.CheckingHashes:
		return CheckingHashes
	default:
		return Downloading
	}
}

func (download *Download) GetProgress() bitfield.Bitfield {
	// TODO: Refactor downloadedPieces.GetStatus.
	downloadStatus := download.downloadedPieces.GetStatus()
	return downloadStatus.Progress
}

func (download *Download) downloadFromAllPeers(
	discoveredPeers <-chan tracker.PeerInfo,
	connectedPeers <-chan connectedPeer,
) {
	ctx, cancel := context.WithCancel(context.Background())

	knownPeers := make([]tracker.PeerInfo, 0)
	for {
		select {
		// TODO: aggregate state changes.
		case pauseState := <-download.setPaused:
			if pauseState {
				cancel()
			} else {
				ctx, cancel = context.WithCancel(context.Background())

				for _, knownPeer := range knownPeers {
					go download.downloadFromPeer(ctx, &knownPeer, nil)
				}
			}
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
				go download.downloadFromPeer(ctx, &newPeer, nil)
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
				go download.downloadFromPeer(ctx, &newPeer.info, newPeer.connection)
			}
		}
	}
}

func (download *Download) downloadFromPeer(
	ctx context.Context,
	peerInfo *tracker.PeerInfo,
	connection *net.Conn,
) {
	for {
		// TODO: Make cancellable.
		peer := peer.Peer{}
		err := peer.Connect(peerInfo, connection)
		if err != nil {
			log.Printf("failed to connect to the peer: %v", err)
			return
		}

		// TODO: Make cancellable.
		err = peer.Handshake(download.torrentInfo)
		if err != nil {
			log.Printf("failed to handshake with the peer: %v", err)
			return
		}

		log.Printf("handshaked with the peer %+v", peerInfo)

		err = peer.StartExchange(ctx, download.torrentInfo, download.Pieces, download.downloadedPieces)
		if err != nil {
			log.Printf("failed to download data from peer: %v. reconnecting", err)
		}

		select {
		case <-ctx.Done():
			return
		default:
			continue
		}
	}
}

type connectedPeer struct {
	info       tracker.PeerInfo
	connection *net.Conn
}

func createTCPListener() (listener net.Listener, listenPort uint16, err error) {
	for i := range listenRetries {
		listenPort = uint16(
			rand.Int()%
				(global_params.ConnectionListenPortMax-global_params.ConnectionListenPortMin+1) +
				global_params.ConnectionListenPortMin)

		newListener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
		if err != nil {
			if i+1 == listenRetries {
				return nil, 0, fmt.Errorf("failed to create TCP listener: %w", err)
			}

			log.Printf("failed to create TCP listener on the port %d; %v", listenPort, err)
		}

		listener = newListener
	}

	return listener, listenPort, nil
}

func (download *Download) acceptConnectionRequests(
	ctx context.Context,
	listener net.Listener,
	connectedPeers chan<- connectedPeer,
) {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

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

		select {
		case connectedPeers <- connectedPeer{info: peerInfo, connection: &conn}:
		case <-ctx.Done():
			return
		}
	}
}
