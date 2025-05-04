package tracker

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
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

type announceRequest struct {
	infoHash   [sha1.Size]byte
	peerID     [20]byte
	downloaded uint64
	left       uint64
	uploaded   uint64
}

func SendRequest(torrent *torrent_info.TorrentInfo) (*TrackerResponse, error) {
	var address = *torrent.Announce

	peerID := [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	announceRequest := announceRequest{
		infoHash:   torrent.InfoHash,
		peerID:     peerID,
		downloaded: 0,
		uploaded:   0,
		left:       uint64(torrent.Length),
	}

	switch address.Scheme {
	case "http":
		return sendHTTPRequest(&address, &announceRequest)
	case "udp":
		return sendUDPRequest(&address, &announceRequest)
	default:
		return nil, fmt.Errorf("unsupported tracker scheme: %s", address.Scheme)
	}
}

func sendHTTPRequest(
	address *url.URL,
	announceRequest *announceRequest,
) (*TrackerResponse, error) {
	Port := 6881

	address.RawQuery = url.Values{
		"info_hash":  []string{string(announceRequest.infoHash[:])},
		"peer_id":    []string{string(announceRequest.peerID[:])},
		"port":       []string{strconv.Itoa(int(Port))},
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

	transcactionID := rand.Uint32()
	connectionID, err := sendUDPConnectionRequest(conn, transcactionID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to send connection request to the UDP tracker %s: %w",
			address.String(),
			err,
		)
	}

	trackerResponse, err := sendUDPAnnounceRequest(conn, transcactionID, connectionID, announceRequest)
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
) (*TrackerResponse, error) {
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
	// 98

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
	// TODO: key
	copy(request[92:96], []byte{0xFF, 0xFF, 0xFF, 0xFF}) // num_want: default: -1
	// TODO: Port

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

	return nil, nil
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
