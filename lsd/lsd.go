package lsd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	"golang.org/x/net/ipv4"
)

const announceInterval = time.Second * 1
const readMessageBufferSize = 2048

func multicastAddressIpv4() netip.AddrPort {
	return netip.AddrPortFrom(netip.AddrFrom4([4]byte{239, 192, 152, 143}), 6771)
}

// TODO: Use.
func multicastAddressIpv6() netip.AddrPort {
	return netip.AddrPortFrom(netip.AddrFrom16(
		[16]byte{0xFF, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xEF, 0xC0, 0x98, 0x8F}),
		6771,
	)
}

func formatRequest(host string, port uint16, infoHashes [][sha1.Size]byte, cookie string) string {
	request := "BT-SEARCH * HTTP/1.1\r\n"
	request += fmt.Sprintf("Host: %s\r\n", host)
	request += fmt.Sprintf("Port: %d\r\n", port)
	for _, infoHash := range infoHashes {
		request += fmt.Sprintf("Infohash: %s\r\n", hex.EncodeToString(infoHash[:]))
	}
	request += fmt.Sprintf("cookie: %s\r\n", cookie)
	request += "\r\n"
	request += "\r\n"

	return request
}

// TODO: Accept multiple info hashes.
func StartDiscovery(infoHash [sha1.Size]byte, errors chan<- error) {
	udpAddr := net.UDPAddrFromAddrPort(multicastAddressIpv4())

	go listenAnnouncements(*udpAddr, errors)

	infoHashes := [1][sha1.Size]byte{infoHash}
	request := formatRequest(udpAddr.String(), 6969, infoHashes[:], "")

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		errors <- fmt.Errorf("UDP dial failed: %w", err)
		return
	}

	for {
		_, err = conn.Write([]byte(request))
		if err != nil {
			errors <- fmt.Errorf("failed to send request: %w", err)
			return
		}

		time.Sleep(announceInterval)
	}
}

// TODO: Listen on all interfaces participating in file exchange.
func listenAnnouncements(address net.UDPAddr, errors chan<- error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		errors <- fmt.Errorf("failed to get network interfaces: %w", err)
		return
	}
	if len(interfaces) == 0 {
		errors <- fmt.Errorf("no network interfaces found: %w", err)
		return
	}

	// TODO: Choose correct interface.
	activeInterface := interfaces[1]

	// TODO: Move to const.
	conn, err := net.ListenPacket("udp", "0.0.0.0:6771")
	if err != nil {
		errors <- fmt.Errorf("failed to create UDP connection: %w", err)
		return
	}

	packetConn := ipv4.NewPacketConn(conn)

	err = packetConn.JoinGroup(&activeInterface, &address)
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

		log.Printf("SOURCE ADDRESS: %v", source)
		log.Printf("MESSAGE: %s", string(buffer[:messageLen]))
	}
}
