package lsd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const announceInterval = time.Second * 60

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

	conn, err := net.ListenMulticastUDP("udp", &interfaces[0], &address)
	if err != nil {
		errors <- fmt.Errorf("failed to listen multicast address: %w", err)
		return
	}

	for {
		buffer := make([]byte, 1024)
		_, err := conn.Read(buffer)
		if err != nil {
			errors <- fmt.Errorf("failed to read from UDP socket: %w", err)
			return
		}
	}
}
