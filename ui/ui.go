package ui

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/pieces"
)

func StartUI(pieces *pieces.Pieces, download *download.Download) {
	mainScreen := tea.NewProgram(mainScreen{pieces: pieces, download: download})
	mainScreen.Run()

	os.Exit(0)
}

type mainScreen struct {
	Width  int
	Height int

	pieces   *pieces.Pieces
	download *download.Download
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

const progressBarPadding int = 5

func (screen mainScreen) View() string {
	downloadedPieces := 0
	totalPieces := screen.pieces.Length()
	for piece := range totalPieces {
		if screen.pieces.GetState(piece) == pieces.Downloaded {
			downloadedPieces++
		}
	}

	var downloadProgressLabel string

	progressBarWidth := screen.Width - progressBarPadding*2
	progressBar := ""

	downloadStatus := screen.download.GetStatus()
	switch downloadStatus.State {
	case download.PreparingFiles:
		downloadProgressLabel = "preparing files"
	case download.CheckingHashes:
		downloadProgressLabel = fmt.Sprintf(
			"checking pieces: %d/%d",
			downloadStatus.Progress,
			downloadStatus.Total,
		)

		progressBar = progress.
			New(progress.WithWidth(progressBarWidth), progress.WithSolidFill("#66F27D")).
			ViewAs(float64(downloadStatus.Progress) / float64(downloadStatus.Total))
	case download.Ready:
		downloadProgressLabel = fmt.Sprintf("downloading: %d/%d", downloadedPieces, totalPieces)
		progressBar = composeDownloadedPiecesString(screen.pieces, progressBarWidth)
	}

	downloadProgressLabel = lipgloss.
		NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#4D756F", Dark: "#A5FAEC"}).
		SetString(downloadProgressLabel).
		Render()
	downloadProgress := lipgloss.
		NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#2E6B38", Dark: "#66F27D"}).
		SetString(progressBar).
		Render()

	return lipgloss.
		NewStyle().
		SetString(
			lipgloss.JoinVertical(lipgloss.Left, downloadProgressLabel, downloadProgress),
		).
		AlignHorizontal(lipgloss.Left).
		AlignVertical(lipgloss.Center).
		Width(screen.Width).
		Height(screen.Height).
		Padding(0, progressBarPadding).
		Render()
}

func composeDownloadedPiecesString(pcs *pieces.Pieces, targetLength int) string {
	pieceCount := pcs.Length()

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
			if pcs.GetState(i) == pieces.Downloaded {
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
