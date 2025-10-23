package tracker

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/mertwole/bittorrent-cli/download/bencode"
)

const maxAnnounceResponseLength = 1024
const udpReadTimeout = time.Second * 20
const minRequestInterval = time.Second * 10

const URLDataOption = 0x2
const EndOfOptions = 0x0

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

type announceRequest struct {
	infoHash   [sha1.Size]byte
	peerID     [20]byte
	downloaded uint64
	left       uint64
	uploaded   uint64
	port       uint16
}

type Tracker struct {
	url      *url.URL
	infoHash [sha1.Size]byte
	length   uint64
	peerID   [20]byte
	interval time.Duration
}

func NewTracker(url *url.URL, infoHash [sha1.Size]byte, length uint64, peerID [20]byte) *Tracker {
	return &Tracker{
		url:      url,
		infoHash: infoHash,
		length:   length,
		peerID:   peerID,
		interval: time.Second * 0,
	}
}

func (tracker *Tracker) ListenForPeers(ctx context.Context, listeningPort uint16, peers chan<- PeerInfo) {
	for {
		select {
		case <-time.After(tracker.interval):
		case <-ctx.Done():
			return
		}

		// TODO: Make cancellable.
		response, err := tracker.sendRequest(listeningPort)
		if err != nil {
			log.Printf("error sending request to the tracker: %v", err)

			if tracker.interval == 0 {
				tracker.interval = time.Second * 60
			}

			continue
		}

		log.Printf("Discovered %d peers", len(response.Peers))

		tracker.interval = time.Second * time.Duration(response.Interval)
		tracker.interval = max(tracker.interval, minRequestInterval)
		for _, peer := range response.Peers {
			select {
			case peers <- peer:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (tracker *Tracker) sendRequest(listenPort uint16) (*TrackerResponse, error) {
	peerID := [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	announceRequest := announceRequest{
		infoHash:   tracker.infoHash,
		peerID:     peerID,
		downloaded: 0,
		uploaded:   0,
		left:       uint64(tracker.length),
		port:       listenPort,
	}

	switch tracker.url.Scheme {
	case "http":
		return sendHTTPRequest(tracker.url, &announceRequest)
	case "https":
		url := *tracker.url
		url.Scheme = "http"

		return sendHTTPRequest(&url, &announceRequest)
	case "udp":
		return sendUDPRequest(tracker.url, &announceRequest)
	default:
		return nil, fmt.Errorf("unsupported tracker scheme: %s", tracker.url.Scheme)
	}
}

func sendHTTPRequest(
	address *url.URL,
	announceRequest *announceRequest,
) (*TrackerResponse, error) {
	address.RawQuery = url.Values{
		"info_hash":  []string{string(announceRequest.infoHash[:])},
		"peer_id":    []string{string(announceRequest.peerID[:])},
		"port":       []string{strconv.Itoa(int(announceRequest.port))},
		"uploaded":   []string{strconv.FormatUint(announceRequest.uploaded, 10)},
		"downloaded": []string{strconv.FormatUint(announceRequest.downloaded, 10)},
		"compact":    []string{"1"},
		"left":       []string{strconv.FormatUint(announceRequest.left, 10)},
	}.Encode()

	response, err := http.Get(address.String())
	if err != nil {
		return nil, fmt.Errorf("failed to send get request to a tracker: %w", err)
	}

	defer response.Body.Close()

	decodedResponse := trackerResponseBencode{}
	err = bencode.Deserialize(response.Body, &decodedResponse)
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

func sendUDPRequest(
	address *url.URL,
	announceRequest *announceRequest,
) (*TrackerResponse, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP tracker address %s: %w", address.String(), err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the UDP tracker %s: %w", address.String(), err)
	}

	err = conn.SetReadDeadline(time.Now().Add(udpReadTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to set UDP read timeout: %w", err)
	}

	transcactionID := rand.Uint32()
	connectionID, err := sendUDPConnectionRequest(conn, transcactionID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to send connection request to the UDP tracker %s: %w",
			address.String(),
			err,
		)
	}

	leadingSlash := ""
	if len(address.Path) != 0 && address.Path[0] != '/' {
		leadingSlash = "/"
	}
	urlData := leadingSlash + address.Path + address.Query().Encode()

	trackerResponse, err := sendUDPAnnounceRequest(
		conn,
		transcactionID,
		connectionID,
		announceRequest,
		urlData,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to send announce request to the UDP tracker %s: %w",
			address.String(),
			err,
		)
	}

	return trackerResponse, nil
}

type connectionID = uint64

func sendUDPConnectionRequest(connection *net.UDPConn, transactionID uint32) (connectionID, error) {
	connectionRequest := make([]byte, 16)
	binary.BigEndian.PutUint64(connectionRequest[:8], 0x41727101980)
	binary.BigEndian.PutUint32(connectionRequest[8:12], 0) // action: connect
	binary.BigEndian.PutUint32(connectionRequest[12:], transactionID)

	_, err := connection.Write(connectionRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}

	response := make([]byte, 16)

	responseBytes, err := connection.Read(response)
	if err != nil {
		return 0, fmt.Errorf("failed to receive response: %w", err)
	}

	if responseBytes < len(response) {
		return 0, fmt.Errorf("invalid response length: length is %d", responseBytes)
	}

	responseAction := binary.BigEndian.Uint32(response[:4])
	if responseAction != 0 {
		return 0, fmt.Errorf("invalid action received: %d, expected 0", responseAction)
	}

	responseTransactionID := binary.BigEndian.Uint32(response[4:8])
	if responseTransactionID != transactionID {
		return 0, fmt.Errorf(
			"invalid transaction ID received: %x, expected %x",
			responseTransactionID,
			transactionID,
		)
	}

	return binary.BigEndian.Uint64(response[8:16]), nil
}

func sendUDPAnnounceRequest(
	connection *net.UDPConn,
	transactionID uint32,
	connectionID uint64,
	announceRequest *announceRequest,
	urlData string,
) (*TrackerResponse, error) {
	var key uint32 = 0xAABBCCDD

	// Offset  Size    			Name    		Value
	// 0       64-bit integer  	connection_id
	// 8       32-bit integer  	action          1 // announce
	// 12      32-bit integer  	transaction_id
	// 16      20-byte string  	info_hash
	// 36      20-byte string  	peer_id
	// 56      64-bit integer  	downloaded
	// 64      64-bit integer  	left
	// 72      64-bit integer  	uploaded
	// 80      32-bit integer  	event           0 // 0: none; 1: completed; 2: started; 3: stopped
	// 84      32-bit integer  	IP address      0 // default
	// 88      32-bit integer  	key
	// 92      32-bit integer  	num_want        -1 // default
	// 96      16-bit integer  	port
	// 98	   Variable			extensions

	request := make([]byte, 98)

	binary.BigEndian.PutUint64(request[:8], connectionID)
	binary.BigEndian.PutUint32(request[8:12], 1) // action: announce
	binary.BigEndian.PutUint32(request[12:16], transactionID)
	copy(request[16:36], announceRequest.infoHash[:])
	copy(request[36:56], announceRequest.peerID[:])
	binary.BigEndian.PutUint64(request[56:64], announceRequest.downloaded)
	binary.BigEndian.PutUint64(request[64:72], announceRequest.left)
	binary.BigEndian.PutUint64(request[72:80], announceRequest.uploaded)
	binary.BigEndian.PutUint32(request[80:84], 0) // event: none
	// TODO: IP address
	binary.BigEndian.PutUint32(request[88:92], key)
	copy(request[92:96], []byte{0xFF, 0xFF, 0xFF, 0xFF}) // num_want: default: -1
	binary.BigEndian.PutUint16(request[96:98], announceRequest.port)

	encodedURLData := encodeURLData(urlData)
	request = append(request, encodedURLData...)

	_, err := connection.Write(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Offset      Size            Name            Value
	// 0           32-bit integer  action          1 // announce
	// 4           32-bit integer  transaction_id
	// 8           32-bit integer  interval
	// 12          32-bit integer  leechers
	// 16          32-bit integer  seeders
	// 20 + 6 * n  32-bit integer  IP address
	// 24 + 6 * n  16-bit integer  TCP port
	// 20 + 6 * N

	response := make([]byte, maxAnnounceResponseLength)
	responseLength, err := connection.Read(response)
	if err != nil {
		return nil, fmt.Errorf("failed to receive response: %w", err)
	}

	if responseLength < 20 || (responseLength-20)%6 != 0 {
		return nil, fmt.Errorf("received response of unexpected length: %d", responseLength)
	}

	responseAction := binary.BigEndian.Uint32(response[:4])
	if responseAction != 1 {
		return nil, fmt.Errorf("unexpected action in response: %d, expected 1", responseAction)
	}

	responseTransactionID := binary.BigEndian.Uint32(response[4:8])
	if responseTransactionID != transactionID {
		return nil, fmt.Errorf(
			"invalid transaction id is received in response: %x, expected %x",
			responseTransactionID,
			transactionID,
		)
	}

	responseInterval := binary.BigEndian.Uint32(response[8:12])

	decodedResponse := &TrackerResponse{
		Interval: int(responseInterval),
		Peers:    make([]PeerInfo, 0),
	}

	responseLeechers := binary.BigEndian.Uint32(response[12:16])
	_ = responseLeechers
	responseSeeders := binary.BigEndian.Uint32(response[16:20])
	_ = responseSeeders

	for peer := range slices.Chunk(response[20:responseLength], 6) {
		peerInfo := PeerInfo{IP: net.IP(peer[:4]), Port: binary.BigEndian.Uint16(peer[4:])}
		decodedResponse.Peers = append(decodedResponse.Peers, peerInfo)
	}

	return decodedResponse, nil
}

func encodeURLData(urlData string) []byte {
	if len(urlData) == 0 {
		return make([]byte, 0)
	}

	result := make([]byte, 2)
	result[0] = URLDataOption
	if len(urlData) >= 256 {
		log.Panicf("unsupported url data length: expected <= 255, got %d", len(urlData))
	}
	result[1] = byte(len(urlData))

	result = append(result, []byte(urlData)...)

	return append(result, EndOfOptions)
}
