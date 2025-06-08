package constants

import "time"

const ConnectionTimeout = time.Second * 120
const KeepAliveInterval = time.Second * 120
const PendingPiecesQueueLength = 5
const RequestedPiecesPopInterval = time.Millisecond * 100
const NotifyPresentPiecesInterval = time.Millisecond * 100
const PieceRequestTimeout = time.Second * 120
const BlockSize = 1 << 14
