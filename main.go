package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mertwole/bittorrent-cli/bitfield"
	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
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

	pieces := pieces.New(len(torrentInfo.Pieces), &donePieces)

	go StartUI(pieces)

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

func StartUI(pieces *pieces.Pieces) {
	mainScreen := tea.NewProgram(mainScreen{pieces: pieces})
	mainScreen.Run()

	os.Exit(0)
}

type mainScreen struct {
	Width  int
	Height int

	pieces *pieces.Pieces
}

func (screen mainScreen) Init() tea.Cmd {
	return tea.Batch(tea.EnterAltScreen, tickCmd())
}

func (screen mainScreen) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "q":
			return screen, tea.Quit
		}
	case tea.WindowSizeMsg:
		screen.Width = message.Width
		screen.Height = message.Height
	case tickMsg:
		return screen, tickCmd()
	}

	return screen, nil
}

func (screen mainScreen) View() string {
	blockCount := screen.Width - 10

	downloadedPiecesBitfield := screen.pieces.GetBitfield()
	str := composeDownloadedPiecesString(&downloadedPiecesBitfield, blockCount)

	downloadedPieces := 0
	totalPieces := screen.pieces.Length()
	for piece := range totalPieces {
		if downloadedPiecesBitfield.ContainsPiece(piece) {
			downloadedPieces++
		}
	}

	downloadProgressLabel := fmt.Sprintf("%d/%d pieces downloaded", downloadedPieces, totalPieces)

	downloadProgressLabel = lipgloss.
		NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#4D756F", Dark: "#A5FAEC"}).
		AlignHorizontal(lipgloss.Left).
		SetString(downloadProgressLabel).
		Render()
	downloadProgress := lipgloss.
		NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#2E6B38", Dark: "#66F27D"}).
		AlignHorizontal(lipgloss.Center).
		SetString(str).
		Render()

	return lipgloss.
		NewStyle().
		SetString(
			lipgloss.JoinVertical(lipgloss.Left, downloadProgressLabel, downloadProgress),
		).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Width(screen.Width).
		Height(screen.Height).
		Render()
}

func composeDownloadedPiecesString(downloadedPieces *bitfield.Bitfield, targetLength int) string {
	pieceCount := downloadedPieces.Length()

	str := ""
	for block := range targetLength {
		pieceToBlockCount := float64(pieceCount) / float64(targetLength)

		firstPiece := int(math.Floor(float64(block) * pieceToBlockCount))
		firstPiece = max(firstPiece, 0)
		lastPiece := int(math.Floor(float64(block+1) * pieceToBlockCount))
		lastPiece = min(lastPiece, pieceCount-1)

		totalPieces := lastPiece - firstPiece + 1
		totalDownloadedPieces := 0
		for i := firstPiece; i <= lastPiece; i++ {
			if downloadedPieces.ContainsPiece(i) {
				totalDownloadedPieces++
			}
		}

		ratio := float64(totalDownloadedPieces) / float64(totalPieces)
		switch {
		case totalDownloadedPieces == totalPieces:
			str += "█"
		case totalDownloadedPieces == 0:
			str += "─"
		case ratio <= 0.33:
			str += "░"
		case ratio <= 0.66:
			str += "▒"
		default:
			str += "▓"
		}
	}

	return str
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func downloadFromPeer(
	peerInfo *tracker.PeerInfo,
	torrentInfo *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
	downloadedPiecesChannel chan<- download.DownloadedPiece,
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

		err = peer.StartDownload(torrentInfo, pieces, downloadedPiecesChannel)
		if err != nil {
			log.Printf("Failed to download data from peer: %v. reconnecting", err)
		}
	}
}
