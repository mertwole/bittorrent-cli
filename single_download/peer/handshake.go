package peer

import (
	"crypto/sha1"
	"fmt"
	"io"
)

type Handshake struct {
	InfoHash [sha1.Size]byte
	PeerID   [20]byte
}

const handshakeLength = 1 + 19 + 8 + sha1.Size + 20
const protocolIdentifier = "BitTorrent protocol"

func (handshake *Handshake) serialize() []byte {
	serialized := make([]byte, handshakeLength)

	// BEP10 extension available
	supportedExtensions := []byte{0, 0, 0, 0, 0, 0x10, 0, 0}

	serialized[0] = 0x13
	copy(serialized[1:20], protocolIdentifier)
	copy(serialized[20:28], supportedExtensions)
	copy(serialized[28:28+sha1.Size], handshake.InfoHash[:])
	copy(serialized[28+sha1.Size:], handshake.PeerID[:])

	return serialized
}

func deserializeHandshake(data io.Reader) (*Handshake, error) {
	protocolNameLength := make([]byte, 1)
	_, err := io.ReadFull(data, protocolNameLength)
	if err != nil {
		return nil, fmt.Errorf("failed to read protocol name length: %w", err)
	}
	if protocolNameLength[0] != 19 {
		return nil, fmt.Errorf("expected first byte to be 19, got %x", protocolNameLength[0])
	}

	protocolIdentifierBytes := make([]byte, 19)
	_, err = io.ReadFull(data, protocolIdentifierBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read protocol identifier: %w", err)
	}
	parsedProtocolIdentifier := string(protocolIdentifierBytes)
	if parsedProtocolIdentifier != protocolIdentifier {
		return nil, fmt.Errorf("invalid protocol identifier: expected %s, got %s", protocolIdentifier, parsedProtocolIdentifier)
	}

	var reserved [8]byte
	_, err = io.ReadFull(data, reserved[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read reserved bytes: %w", err)
	}

	var infoHash [sha1.Size]byte
	_, err = io.ReadFull(data, infoHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read info hash: %w", err)
	}

	var peerID [20]byte
	_, err = io.ReadFull(data, peerID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read peer ID: %w", err)
	}

	return &Handshake{
		InfoHash: infoHash,
		PeerID:   peerID,
	}, nil
}
