package lsd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/mertwole/bittorrent-cli/global_params"
	"github.com/mertwole/bittorrent-cli/single_download/tracker"
	"golang.org/x/net/ipv4"
)

// TODO: Set a bigger time as specified in the BEP14.
const announceInterval = time.Second * 5
const readMessageBufferSize = 2048
const multicastPort = 6771

func multicastAddressIpv4() netip.AddrPort {
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{239, 192, 152, 143}), multicastPort)
}

// TODO: Use.
func multicastAddressIpv6() netip.AddrPort {
	return netip.AddrPortFrom(netip.AddrFrom16(
		[16]byte{0xFF, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xEF, 0xC0, 0x98, 0x8F}),
		multicastPort,
	)
}

// TODO: Accept multiple info hashes.
func StartDiscovery(infoHash [sha1.Size]byte, discoveredPeers chan<- tracker.PeerInfo, errors chan<- error) {
	udpAddr := net.UDPAddrFromAddrPort(multicastAddressIpv4())

	cookie := strconv.FormatInt(rand.Int64(), 36)

	interfaces, err := net.Interfaces()
	if err != nil {
		errors <- fmt.Errorf("failed to get network interfaces: %w", err)
		return
	}

	listeningOnAny := false
	for _, listenInterface := range interfaces {
		if listenInterface.Flags&net.FlagMulticast == 0 {
			continue
		}

		listeningOnAny = true
		go listenAnnouncements(*udpAddr, infoHash, cookie, listenInterface, discoveredPeers, errors)
	}

	if !listeningOnAny {
		log.Printf("no interfaces supporting multicast are found. cannot start LSD")
		return
	}

	infoHashes := [1][sha1.Size]byte{infoHash}
	message := btSearchMessage{
		host:       udpAddr.String(),
		port:       global_params.ConnectionListenPort,
		infoHashes: infoHashes[:],
		cookie:     cookie,
	}
	request := formatMessage(message)

	// TODO: Announce on all interfaces?
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		errors <- fmt.Errorf("UDP dial failed: %w", err)
		return
	}

	for {
		time.Sleep(announceInterval)

		_, err = conn.Write([]byte(request))
		if err != nil {
			errors <- fmt.Errorf("failed to send request: %w", err)
			return
		}
	}
}

func listenAnnouncements(
	address net.UDPAddr,
	requiredInfoHash [sha1.Size]byte,
	cookie string,
	listenInterface net.Interface,
	discoveredPeers chan<- tracker.PeerInfo,
	errors chan<- error,
) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", multicastPort))
	if err != nil {
		errors <- fmt.Errorf("failed to create UDP connection: %w", err)
		return
	}

	packetConn := ipv4.NewPacketConn(conn)

	err = packetConn.JoinGroup(&listenInterface, &address)
	if err != nil {
		errors <- fmt.Errorf("failed to join multicast group: %w", err)
		return
	}

	err = packetConn.SetControlMessage(ipv4.FlagDst, true)
	if err != nil {
		errors <- fmt.Errorf("failed to set control message: %w", err)
		return
	}

	for {
		buffer := make([]byte, readMessageBufferSize)
		messageLen, _, source, err := packetConn.ReadFrom(buffer)

		if err != nil {
			errors <- fmt.Errorf("failed to read UDP message: %w", err)
			return
		}

		message, err := parseMessage(string(buffer[:messageLen]))
		if err != nil {
			fmt.Printf("failed to read btsearch response: %v", err)
			continue
		}

		if message.cookie == cookie {
			continue
		}

		infoHashFound := false
		for _, infoHash := range message.infoHashes {
			if infoHash == requiredInfoHash {
				infoHashFound = true
				break
			}
		}

		if !infoHashFound {
			continue
		}

		// TODO: Validate message.host?

		sourceAddress := source.String()
		sourceAddrPort, err := netip.ParseAddrPort(sourceAddress)
		if err != nil {
			log.Panicf("unable to parse address and port: %v", err)
		}
		sourceIP := sourceAddrPort.Addr().As4()

		discoveredPeers <- tracker.PeerInfo{IP: sourceIP[:], Port: message.port}
	}
}

type btSearchMessage struct {
	host       string
	port       uint16
	infoHashes [][sha1.Size]byte
	cookie     string
}

const (
	portHeader     = "Port: "
	cookieHeader   = "cookie: "
	infohashHeader = "Infohash: "
	hostHeader     = "Host: "
)

func formatMessage(message btSearchMessage) string {
	messageString := "BT-SEARCH * HTTP/1.1\r\n"
	messageString += fmt.Sprintf("%s%s\r\n", hostHeader, message.host)
	messageString += fmt.Sprintf("%s%d\r\n", portHeader, message.port)
	for _, infoHash := range message.infoHashes {
		messageString += fmt.Sprintf("%s%s\r\n", infohashHeader, hex.EncodeToString(infoHash[:]))
	}
	messageString += fmt.Sprintf("%s%s\r\n", cookieHeader, message.cookie)
	messageString += "\r\n"
	messageString += "\r\n"

	return messageString
}

func parseMessage(messageString string) (btSearchMessage, error) {
	message := btSearchMessage{}
	message.infoHashes = make([][sha1.Size]byte, 0)

	lines := strings.SplitSeq(messageString, "\r\n")

	for line := range lines {
		if remaining, ok := trimPrefix(line, portHeader); ok {
			port, err := strconv.ParseUint(remaining, 10, 16)
			if err != nil {
				return message, fmt.Errorf("failed to parse port: %w", err)
			}

			message.port = uint16(port)
		} else if remaining, ok = trimPrefix(line, cookieHeader); ok {
			message.cookie = remaining
		} else if remaining, ok = trimPrefix(line, infohashHeader); ok {
			infoHash, err := hex.DecodeString(remaining)
			if err != nil {
				return message, fmt.Errorf("failed to decode infohash: %w", err)
			}
			if len(infoHash) != sha1.Size {
				return message, fmt.Errorf(
					"received infohash of invalid length: expected %d, got %d",
					sha1.Size,
					len(infoHash),
				)
			}

			message.infoHashes = append(message.infoHashes, [20]byte(infoHash))
		} else if remaining, ok = trimPrefix(line, hostHeader); ok {
			message.host = remaining
		}
	}

	return message, nil
}

func trimPrefix(str, prefix string) (remaining string, ok bool) {
	if len(str) < len(prefix) {
		return "", false
	}

	if str[:len(prefix)] != prefix {
		return "", false
	}

	return str[len(prefix):], true
}
