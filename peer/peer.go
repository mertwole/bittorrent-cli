package peer

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/mertwole/bittorent-cli/torrent_info"
	"github.com/mertwole/bittorent-cli/tracker"
)

type Peer struct {
	Info       tracker.PeerInfo
	Connection net.Conn
}

func (peer *Peer) Connect(info *tracker.PeerInfo) error {
	peer.Info = *info

	conn, err := net.Dial("tcp", info.IP.String()+":"+strconv.Itoa(int(info.Port)))
	if err != nil {
		return fmt.Errorf("failed to establish connection with peer %s: %w", info.IP.String(), err)
	}

	peer.Connection = conn

	return nil
}

func (peer *Peer) Handshake(torrent *torrent_info.TorrentInfo) error {
	handshake := handshake{
		PeerID:   [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
		InfoHash: torrent.InfoHash,
	}
	serializedHandshake := handshake.serialize()

	_, err := peer.Connection.Write(serializedHandshake)
	if err != nil {
		return fmt.Errorf("failed to send request to the peer %s: %w", peer.Info.IP.String(), err)
	}

	response, err := io.ReadAll(peer.Connection)
	if err != nil {
		return fmt.Errorf("failed to get response from the peer %s: %w", peer.Info.IP.String(), err)
	}

	responseHandshake, err := deserializeHandshake(response)
	if err != nil {
		return fmt.Errorf("failed to decode handshake from peer %s: %w", peer.Info.IP.String(), err)
	}

	var _ = responseHandshake

	return nil
}

type handshake struct {
	InfoHash [sha1.Size]byte
	PeerID   [20]byte
}

const handshakeLength = 1 + 19 + 8 + sha1.Size + 20
const protocolIdentifier = "BitTorrent protocol"

func (handshake *handshake) serialize() []byte {
	serialized := make([]byte, handshakeLength)

	serialized[0] = 0x13
	copy(serialized[1:20], protocolIdentifier)
	copy(serialized[20:28], []byte{0, 0, 0, 0, 0, 0, 0, 0})
	copy(serialized[28:28+sha1.Size], handshake.InfoHash[:])
	copy(serialized[28+sha1.Size:], handshake.PeerID[:])

	return serialized
}

func deserializeHandshake(data []byte) (*handshake, error) {
	if len(data) < handshakeLength {
		return nil, fmt.Errorf("invalid length: expected %d, got %d", handshakeLength, len(data))
	}

	if data[0] != 0x13 {
		return nil, fmt.Errorf("expected first byte to be 0x13, got %x", data[0])
	}

	parsedProtocolIdentifier := string(data[1:20])
	if parsedProtocolIdentifier != protocolIdentifier {
		return nil, fmt.Errorf("invalid protocol identifier: expected %s, got %s", protocolIdentifier, parsedProtocolIdentifier)
	}

	return &handshake{
		InfoHash: [sha1.Size]byte(data[28 : 28+sha1.Size]),
		PeerID:   [20]byte(data[28+sha1.Size : 28+sha1.Size+20]),
	}, nil
}
