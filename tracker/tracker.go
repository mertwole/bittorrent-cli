package tracker

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/jackpal/bencode-go"
	"github.com/mertwole/bittorent-cli/torrent_info"
)

type TrackerResponse struct {
	Interval int
	Peers    []PeerInfo
}

type PeerInfo struct {
	IP   net.IP
	Port uint16
}

type trackerResponseBencode struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func SendRequest(torrent *torrent_info.TorrentInfo) (*TrackerResponse, error) {
	var address = *torrent.Announce

	peerID := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	Port := 6881

	address.RawQuery = url.Values{
		"info_hash":  []string{string(torrent.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(Port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(torrent.Length)},
	}.Encode()

	response, err := http.Get(address.String())
	if err != nil {
		return nil, fmt.Errorf("failed to send get request to a tracker: %w", err)
	}

	defer response.Body.Close()

	decodedResponse := trackerResponseBencode{}
	err = bencode.Unmarshal(response.Body, &decodedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %w", err)
	}

	peers, err := decodePeerInfo(&decodedResponse.Peers)
	if err != nil {
		return nil, fmt.Errorf("failed to decode peer info: %w", err)
	}

	return &TrackerResponse{
		Interval: decodedResponse.Interval,
		Peers:    peers,
	}, nil
}

func decodePeerInfo(peers *string) ([]PeerInfo, error) {
	if len(*peers)%6 != 0 {
		return nil, fmt.Errorf("invalid peer list format")
	}

	peerInfos := make([]PeerInfo, 0)
	for info := range slices.Chunk([]byte(*peers), 6) {
		peerInfos = append(peerInfos, PeerInfo{
			IP:   net.IP(info[:4]),
			Port: binary.BigEndian.Uint16(info[4:]),
		})
	}

	return peerInfos, nil
}
