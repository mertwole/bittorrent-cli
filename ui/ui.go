package ui

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mertwole/bittorrent-cli/bitfield"
	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/single_download"
)

const torrentFileExtension = ".torrent"
const updateDownloadedPiecesPollInterval = time.Millisecond * 1000

func StartUI() {
	//runtime.LockOSThread()

	download_1, _ := single_download.New("./data/lc.torrent", "./data")
	download_2, _ := single_download.New("./data/oni.torrent", "./data")
	download_3, _ := single_download.New("./data/debian.torrent", "./data")

	go download_1.Start()
	go download_2.Start()
	go download_3.Start()

	downloadList := []list.Item{
		downloadItem{model: download_1, downloadedPieces: bitfield.NewEmptyConcurrentBitfield(0)},
		downloadItem{model: download_2, downloadedPieces: bitfield.NewEmptyConcurrentBitfield(0)},
		downloadItem{model: download_3, downloadedPieces: bitfield.NewEmptyConcurrentBitfield(0)},
	}

	list := list.New(downloadList, downloadItemDelegate{}, 20, 20)
	list.Title = "downloads"
	list.SetFilteringEnabled(false)
	list.SetShowStatusBar(false)

	filePicker := filepicker.New()
	filePicker.AllowedTypes = []string{torrentFileExtension}
	filePicker.CurrentDirectory, _ = os.UserHomeDir()

	mainScreen := tea.NewProgram(mainScreen{downloadList: &list, filePicker: &filePicker})
	mainScreen.Run()

	os.Exit(0)
}

type mainScreen struct {
	Width  int
	Height int

	downloadList *list.Model
	filePicker   *filepicker.Model

	additionRequest *additionRequest
}

type additionRequest struct {
	filePath string
}

func (screen mainScreen) Init() tea.Cmd {
	filePickerCmd := screen.filePicker.Init()

	go screen.updateDownloadedPieces()

	return tea.Batch(tea.EnterAltScreen, tickCmd(), filePickerCmd)
}

func (screen *mainScreen) updateDownloadedPieces() {
	for {
		bitfields := make([]bitfield.Bitfield, 0)

		for _, item := range screen.downloadList.Items() {
			downloadItem, ok := item.(downloadItem)
			if !ok {
				continue
			}

			bitfields = append(bitfields, downloadItem.model.Pieces.GetBitfield())
		}

		for i, item := range screen.downloadList.Items() {
			downloadItem, ok := item.(downloadItem)
			if !ok {
				continue
			}

			downloadItem.downloadedPieces.SetBitfield(bitfields[i])
		}

		time.Sleep(updateDownloadedPiecesPollInterval)
	}
}

func (screen mainScreen) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "q":
			return screen, tea.Quit
		case "left":
			screen.downloadList.PrevPage()
			return screen, nil
		case "right":
			screen.downloadList.NextPage()
			return screen, nil
		case "+":
			screen.additionRequest = &additionRequest{}
			return screen, nil
		}
	case tea.WindowSizeMsg:
		screen.Width = message.Width
		screen.Height = message.Height

		log.Printf("NEW SIZE: %d %d", message.Width, message.Height)

		return screen, nil
	case tickMsg:
		return screen, tickCmd()
	}

	var downloadListCmd tea.Cmd
	var filePickerCmd tea.Cmd

	*screen.downloadList, downloadListCmd = screen.downloadList.Update(message)
	*screen.filePicker, filePickerCmd = screen.filePicker.Update(message)

	return screen, tea.Batch(downloadListCmd, filePickerCmd)
}

func (screen mainScreen) View() string {
	if screen.additionRequest != nil {
		return screen.filePicker.View()
	} else {
		startTime := time.Now()

		log.Printf("View time(before): %v", time.Since(startTime))

		log.Printf("RENDERED WITH SIZE: %d %d", screen.Width, screen.Height)

		screen.downloadList.SetSize(screen.Width, screen.Height)

		res := screen.downloadList.View()

		log.Printf("View time: %v", time.Since(startTime))

		return res
	}
}

type downloadItem struct {
	model            *single_download.Download
	downloadedPieces *bitfield.ConcurrentBitfield
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
	totalPieces := item.downloadedPieces.PieceCount()
	piecesBitfield := item.downloadedPieces.GetBitfield()
	for piece := range totalPieces {
		if piecesBitfield.ContainsPiece(piece) {
			downloadedPieces++
		}
	}

	var downloadProgressLabel string

	progressBarWidth := m.Width() - 5
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
		progressBar = composeDownloadedPiecesString(item.downloadedPieces, progressBarWidth)
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

func composeDownloadedPiecesString(bitfield *bitfield.ConcurrentBitfield, targetLength int) string {
	pieceCount := bitfield.PieceCount()

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
			if bitfield.ContainsPiece(i) {
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
	return tea.Tick(time.Millisecond*10, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
