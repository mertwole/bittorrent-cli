package main

import (
	"flag"
	"log"
	"os"

	"github.com/mertwole/bittorrent-cli/single_download"
	"github.com/mertwole/bittorrent-cli/ui"
)

const logFileName = "log"

var torrentFileName = flag.String("torrent", "./data/torrent.torrent", "Path to the .torrent file")
var downloadFolderName = flag.String("download", "./data", "Path to the download folder")
var interactiveMode = flag.Bool("interactive", true, "Whether the client should be run in an interactive mode")

func main() {
	flag.Parse()

	if *interactiveMode {
		logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			log.Fatalf("error opening log file: %v", err)
		}
		defer logFile.Close()

		log.SetOutput(logFile)
	}

	pieces, downloadedPieces, err := single_download.StartDownload(*torrentFileName, *downloadFolderName)
	if err != nil {
		log.Fatalf("failed to start download: %v", err)
	}

	if *interactiveMode {
		ui.StartUI(pieces, downloadedPieces)
	}
}
