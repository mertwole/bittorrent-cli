package tracker

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand/v2"
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

	switch address.Scheme {
	case "http":
		return sendHTTPRequest(&address, peerID, torrent)
	case "udp":
		return sendUDPRequest(&address, peerID, torrent)
	default:
		return nil, fmt.Errorf("unsupported tracker scheme: %s", address.Scheme)
	}
}

func sendHTTPRequest(
	address *url.URL,
	peerID []byte,
	torrent *torrent_info.TorrentInfo,
) (*TrackerResponse, error) {
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

func sendUDPRequest(
	address *url.URL,
	peerID []byte,
	torrent *torrent_info.TorrentInfo,
) (*TrackerResponse, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP tracker address %s: %w", address.String(), err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the UDP tracker %s: %w", address.String(), err)
	}

	transcactionID := rand.Uint32()
	connectionID, err := sendUDPConnectionRequest(conn, transcactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the UDP tracker %s: %w", address.String(), err)
	}

	log.Printf("connection ID: %x", connectionID)

	return nil, nil
}

type connectionID = uint64

func sendUDPConnectionRequest(connection *net.UDPConn, transactionID uint32) (connectionID, error) {
	connectionRequest := make([]byte, 16)
	binary.BigEndian.PutUint64(connectionRequest[:8], 0x41727101980)
	binary.BigEndian.PutUint32(connectionRequest[8:12], 0) // action: connect
	binary.BigEndian.PutUint32(connectionRequest[12:], transactionID)

	_, err := connection.Write(connectionRequest)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to send connection request to the UDP tracker %s: %w",
			connection.RemoteAddr().String(),
			err,
		)
	}

	response := make([]byte, 16)

	responseBytes, err := connection.Read(response)
	if err != nil {
		return 0, fmt.Errorf(
			"failed to receive response from the UDP tracker %s: %w",
			connection.RemoteAddr().String(),
			err,
		)
	}

	if responseBytes < len(response) {
		return 0, fmt.Errorf(
			"connection response with invalid length is received from the UDP tracker %s: length is %d",
			connection.RemoteAddr().String(),
			responseBytes,
		)
	}

	responseAction := binary.BigEndian.Uint32(response[:4])
	if responseAction != 0 {
		return 0, fmt.Errorf("invalid action in UDP tracker response: %d, expected 0", responseAction)
	}

	responseTransactionID := binary.BigEndian.Uint32(response[4:8])
	if responseTransactionID != transactionID {
		return 0, fmt.Errorf(
			"invalid transaction ID in UDP tracker response: %x, expected %x",
			responseTransactionID,
			transactionID,
		)
	}

	return binary.BigEndian.Uint64(response[8:16]), nil
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
