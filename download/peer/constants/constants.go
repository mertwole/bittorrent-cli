package constants

import (
	"log"
	"time"

	"github.com/mertwole/bittorrent-cli/download/peer/extensions"
)

const ConnectionTimeout = time.Second * 120
const KeepAliveInterval = time.Second * 120
const RequestedPiecesPopInterval = time.Millisecond * 100
const NotifyPresentPiecesInterval = time.Millisecond * 100
const PieceRequestTimeout = time.Second * 120
const CancelMessagesSendInterval = time.Millisecond * 100
const BlockSize = 1 << 14
const PendingPiecesQueueLength = 5

func SupportedExtensions() extensions.Extensions {
	supported := []string{""}

	extensions, err := extensions.New(supported)
	if err != nil {
		log.Panicf("failed to create supported extensions: %v", err)
	}

	return extensions
}
