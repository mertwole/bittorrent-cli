package ui

import (
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/single_download"
)

type downloadItem struct {
	model *single_download.Download
}

func (i downloadItem) FilterValue() string { return "" }

type downloadItemDelegate struct{}

func (d downloadItemDelegate) Height() int {
	return 2
}

func (d downloadItemDelegate) Spacing() int {
	return 1
}

func (d downloadItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d downloadItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(downloadItem)
	if !ok {
		return
	}

	model := item.model

	downloadedPieces := 0
	totalPieces := model.Pieces.Length()
	for piece := range totalPieces {
		if model.Pieces.GetState(piece) == pieces.Downloaded {
			downloadedPieces++
		}
	}

	var downloadProgressLabel string

	progressBarWidth := 20 // TODO  screen.Width - progressBarPadding*2
	progressBar := ""

	downloadStatus := model.DownloadedPieces.GetStatus()
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
		progressBar = composeDownloadedPiecesString(model.Pieces, progressBarWidth)
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

	fmt.Fprintf(w, "%s\n%s", downloadProgressLabel, downloadProgress)
}

func StartUI() {
	download_1, _ := single_download.New("./data/lc.torrent", "./data")
	download_2, _ := single_download.New("./data/kcd.torrent", "./data")
	download_3, _ := single_download.New("./data/debian.torrent", "./data")

	go download_1.Start()
	go download_2.Start()
	go download_3.Start()

	downloadList := []list.Item{
		downloadItem{model: download_1},
		downloadItem{model: download_2},
		downloadItem{model: download_3},
	}

	list := list.New(downloadList, downloadItemDelegate{}, 20, 20)

	mainScreen := tea.NewProgram(mainScreen{downloadList: list})
	mainScreen.Run()

	os.Exit(0)
}

type mainScreen struct {
	Width  int
	Height int

	downloadList list.Model
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
	return screen.downloadList.View()
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
