package magnet_link

import (
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
)

const scheme = "magnet"
const infoHashPrefix = "urn:btih:"
const taggedInfoHashPrefix = "urn:btmh:"

type Data struct {
	InfoHash [sha1.Size]byte
	Trackers []*url.URL
}

func Decode(link string) (*Data, error) {
	uri, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	if uri.Scheme != scheme {
		return nil, fmt.Errorf("invalid scheme: %s", uri.Scheme)
	}

	query := uri.Query()

	infoHashes, ok := query["xt"]
	if !ok || len(infoHashes) == 0 {
		return nil, fmt.Errorf("expected at least one info hash in query")
	}
	if len(infoHashes) != 1 {
		log.Printf("unexpected amount of info hashes found: %d", len(infoHashes))
	}

	var parsedInfoHash [sha1.Size]byte

	infoHash := infoHashes[0]
	switch {
	case strings.HasPrefix(infoHash, infoHashPrefix):
		infoHash = infoHash[len(infoHashPrefix):]

		if len(infoHash) == sha1.Size*2 {
			decoded, err := hex.DecodeString(infoHash)
			if err != nil {
				return nil, fmt.Errorf("invalid hex info hash %s", infoHash)
			}

			parsedInfoHash = [sha1.Size]byte(decoded)
		} else {
			decoded, err := base32.StdEncoding.DecodeString(infoHash)
			if err != nil {
				return nil, fmt.Errorf("invalid base32 info hash %s", infoHash)
			}

			if len(decoded) != sha1.Size {
				return nil, fmt.Errorf("invalid info hash length: expected %d, got %d", sha1.Size, len(decoded))
			}

			parsedInfoHash = [sha1.Size]byte(decoded)
		}
	case strings.HasPrefix(infoHash, taggedInfoHashPrefix):
		// TODO: Decode tagged info hash.

		log.Panicf("tagged hash decoding is not implemented")
	default:
		return nil, fmt.Errorf(
			"invalid info hash prefix: %s, expected one of urn:btih: or urn:btmh: ",
			infoHash,
		)
	}

	trackers, ok := query["tr"]
	trackerUrls := make([]*url.URL, 0)

	if ok {
		for _, tracker := range trackers {
			trackerURL, err := url.Parse(tracker)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tracker URL %s: %w", tracker, err)
			}

			trackerUrls = append(trackerUrls, trackerURL)
		}
	}

	return &Data{Trackers: trackerUrls, InfoHash: parsedInfoHash}, nil
}
