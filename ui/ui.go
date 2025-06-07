package ui

import (
	"fmt"
	"math"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mertwole/bittorrent-cli/bitfield"
	"github.com/mertwole/bittorrent-cli/pieces"
)

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
	pieceCount := downloadedPieces.PieceCount()

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
