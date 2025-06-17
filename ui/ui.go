package ui

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/download/bitfield"
	"github.com/mertwole/bittorrent-cli/download/downloaded_files"
)

const torrentFileExtension = ".torrent"
const updateDownloadedPiecesPollInterval = time.Millisecond * 100

func StartUI() {
	keyMap := defaultKeyMap()

	newList := list.New(make([]list.Item, 0), downloadItemDelegate{}, 20, 20)
	newList.SetShowTitle(false)
	newList.SetFilteringEnabled(false)
	newList.SetShowStatusBar(false)
	newList.SetShowHelp(false)

	newList.KeyMap = list.KeyMap{
		CursorUp:   keyMap.moveUp,
		CursorDown: keyMap.moveDown,
		NextPage:   keyMap.nextPage,
		PrevPage:   keyMap.previousPage,
	}

	filePicker := filepicker.New()
	filePicker.AllowedTypes = []string{torrentFileExtension}
	filePicker.CurrentDirectory, _ = os.UserHomeDir()
	filePicker.AutoHeight = true

	mainScreen := tea.NewProgram(mainScreen{
		downloadList:    &newList,
		filePicker:      &filePicker,
		keyMap:          keyMap,
		help:            help.New(),
		additionRequest: false,
	})
	mainScreen.Run()

	os.Exit(0)
}

type mainScreen struct {
	Width  int
	Height int

	downloadList *list.Model
	filePicker   *filepicker.Model

	keyMap keyMap
	help   help.Model

	additionRequest bool
}

func (screen mainScreen) Init() tea.Cmd {
	go screen.updateDownloadedPieces()

	return tea.Batch(tea.EnterAltScreen, tickCmd())
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
	command := tea.Batch()

	var downloadListCmd tea.Cmd
	*screen.downloadList, downloadListCmd = screen.downloadList.Update(message)
	command = tea.Batch(command, downloadListCmd)

	var paginatorCmd tea.Cmd
	screen.downloadList.Paginator, paginatorCmd = screen.downloadList.Paginator.Update(message)
	command = tea.Batch(command, paginatorCmd)

	var filePickerCmd tea.Cmd
	*screen.filePicker, filePickerCmd = screen.filePicker.Update(message)
	command = tea.Batch(command, filePickerCmd)

	switch message := message.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(message, screen.keyMap.quit):
			command = tea.Batch(command, tea.Quit)
		case key.Matches(message, screen.keyMap.toggleHelp):
			screen.help.ShowAll = !screen.help.ShowAll
		case key.Matches(message, screen.keyMap.addTorrent):
			screen.additionRequest = true
			filePickerCmd := screen.filePicker.Init()
			command = tea.Batch(command, filePickerCmd)
		}
	case tea.WindowSizeMsg:
		screen.Width = message.Width
		screen.Height = message.Height
	case tickMsg:
		command = tea.Batch(tickCmd())
	}

	didSelect, filePath := screen.filePicker.DidSelectFile(message)
	if didSelect {
		screen.additionRequest = false

		// TODO: Determine download path.
		newDownload, err := download.New(filePath, "./data")
		if err != nil {
			// TODO: Show this error to the user.
			log.Panicf("failed to add file to downloads: %v", err)
		}

		go newDownload.Start()

		newItem := downloadItem{
			model:            newDownload,
			downloadedPieces: bitfield.NewEmptyConcurrentBitfield(0),
		}
		// TODO: Check if it's not duplicate.
		screen.downloadList.InsertItem(math.MaxInt, newItem)
	}

	return screen, command
}

func (screen mainScreen) View() string {
	if screen.additionRequest {
		return screen.filePicker.View()
	} else {
		screen.help.Width = screen.Width

		help := screen.help.View(screen.keyMap)
		helpHeight := lipgloss.Height(help)

		screen.downloadList.SetSize(screen.Width, screen.Height-helpHeight)

		return screen.downloadList.View() + "\n" + help
	}
}

type downloadItem struct {
	model            *download.Download
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

	totalWidth := m.Width()

	if index == m.Index() {
		totalWidth -= 2
	}

	progressBarWidth := totalWidth
	progressBar := ""

	downloadStatus := model.DownloadedPieces.GetStatus()
	switch downloadStatus.State {
	case downloaded_files.PreparingFiles:
		downloadProgressLabel = "preparing files"
	case downloaded_files.CheckingHashes:
		downloadProgressLabel = "checking files"

		progressBar = progress.
			New(progress.WithWidth(progressBarWidth), progress.WithSolidFill("#66F27D")).
			ViewAs(float64(downloadStatus.Progress) / float64(downloadStatus.Total))
	case downloaded_files.Ready:
		downloadProgressLabel = "downloading"

		downloadPercent := float64(downloadedPieces) / float64(totalPieces) * 100.

		maxPercentageLength := len(" 100.0%")
		progressBarWidth -= maxPercentageLength

		progress := fmt.Sprintf("%.1f%%", downloadPercent)
		progress = fmt.Sprintf("%*s", maxPercentageLength, progress)

		progressBar = composeDownloadedPiecesString(item.downloadedPieces, progressBarWidth)
		progressBar += progress
	}

	nameLabel := model.GetTorrentName()

	paddingLength := totalWidth - lipgloss.Width(nameLabel)
	statusLabel := fmt.Sprintf("%s%*s", nameLabel, paddingLength, downloadProgressLabel)

	if index == m.Index() {
		statusLabel = "┆ " + statusLabel
		progressBar = "┆ " + progressBar
	}

	statusLabel = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#4D756F", Dark: "#A5FAEC"}).
		Render(statusLabel)

	downloadProgress := lipgloss.
		NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#2E6B38", Dark: "#66F27D"}).
		SetString(progressBar).
		Render()

	fmt.Fprintf(w, "%s\n%s", statusLabel, downloadProgress)
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
