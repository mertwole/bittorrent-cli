package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	moveUp       key.Binding
	moveDown     key.Binding
	nextPage     key.Binding
	previousPage key.Binding

	addTorrent          key.Binding
	pauseUnpauseTorrent key.Binding
	removeTorrent       key.Binding

	toggleHelp key.Binding

	quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.toggleHelp, k.quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.moveUp, k.moveDown, k.nextPage, k.previousPage},
		{k.addTorrent, k.pauseUnpauseTorrent, k.removeTorrent},
		{k.toggleHelp, k.quit},
	}
}

func defaultKeyMap() keyMap {
	return keyMap{
		moveUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "move up in list"),
		),
		moveDown: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "move down in list"),
		),
		nextPage: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "next page"),
		),
		previousPage: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "previous page"),
		),
		addTorrent: key.NewBinding(
			key.WithKeys("+"),
			key.WithHelp("+", "add torrent"),
		),
		pauseUnpauseTorrent: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause/unpause selected torrent"),
		),
		removeTorrent: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "remove selected torrent"),
		),
		toggleHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
