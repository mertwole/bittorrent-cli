package peer

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/mertwole/bittorrent-cli/bitfield"
	"github.com/mertwole/bittorrent-cli/download"
	"github.com/mertwole/bittorrent-cli/peer/constants"
	"github.com/mertwole/bittorrent-cli/peer/extensions"
	"github.com/mertwole/bittorrent-cli/peer/message"
	"github.com/mertwole/bittorrent-cli/peer/pending_pieces"
	"github.com/mertwole/bittorrent-cli/peer/requested_pieces"
	"github.com/mertwole/bittorrent-cli/pieces"
	"github.com/mertwole/bittorrent-cli/torrent_info"
	"github.com/mertwole/bittorrent-cli/tracker"
)

func supportedExtensions() extensions.Extensions {
	supported := []string{""}

	extensions, err := extensions.New(supported)
	if err != nil {
		log.Panicf("failed to create supported extensions: %v", err)
	}

	return extensions
}

type Peer struct {
	info       tracker.PeerInfo
	clientName string

	connection net.Conn

	chocked             bool
	availablePieces     *bitfield.ConcurrentBitfield
	availableExtensions extensions.Extensions

	pendingPieces   pending_pieces.PendingPieces
	requestedPieces requested_pieces.RequestedPieces

	pieces *pieces.Pieces
}

func (peer *Peer) GetInfo() tracker.PeerInfo {
	return peer.info
}

func (peer *Peer) Connect(info *tracker.PeerInfo) error {
	peer.info = *info
	peer.chocked = true
	peer.availableExtensions = extensions.Empty()

	conn, err := net.DialTimeout(
		"tcp",
		info.IP.String()+":"+strconv.Itoa(int(info.Port)),
		constants.ConnectionTimeout,
	)
	if err != nil {
		return fmt.Errorf("failed to establish connection with peer %s: %w", info.IP.String(), err)
	}

	peer.connection = conn

	return nil
}

func (peer *Peer) Handshake(torrent *torrent_info.TorrentInfo) error {
	handshake := Handshake{
		PeerID:   [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0},
		InfoHash: torrent.InfoHash,
	}
	serializedHandshake := handshake.serialize()

	_, err := peer.connection.Write(serializedHandshake)
	if err != nil {
		return fmt.Errorf("failed to send request to the peer %s: %w", peer.info.IP.String(), err)
	}

	responseHandshake, err := deserializeHandshake(peer.connection)
	if err != nil {
		return fmt.Errorf("failed to decode handshake from peer %s: %w", peer.info.IP.String(), err)
	}

	if responseHandshake.InfoHash != torrent.InfoHash {
		return fmt.Errorf(
			"invalid info hash received from the peer %s: expected %v, got %v",
			peer.info.IP.String(),
			torrent.InfoHash,
			responseHandshake.InfoHash,
		)
	}

	// TODO: Check if extension protocol(BEP10) is supported.

	supportedExtensions := supportedExtensions()
	extendedHandshake := message.ExtendedHandshake{SupportedExtensions: supportedExtensions.GetMapping()}

	_, err = peer.connection.Write(extendedHandshake.Encode())
	if err != nil {
		return fmt.Errorf("failed to send extended handshake to the peer %s: %w", peer.info.IP.String(), err)
	}

	return nil
}

func (peer *Peer) StartExchange(
	torrent *torrent_info.TorrentInfo,
	pieces *pieces.Pieces,
	downloadedPieces *download.Download,
) error {
	// TODO: Cancel goroutines when error occured and cleanup the pendingPieces.

	peer.pieces = pieces
	peer.pendingPieces = pending_pieces.NewPendingPieces()
	peer.availablePieces = bitfield.NewEmptyConcurrentBitfield(len(torrent.Pieces))

	err := peer.sendInitialMessages()
	if err != nil {
		return fmt.Errorf("failed to send initial messages: %w", err)
	}

	notifyPresentPiecesErrors := make(chan error)
	go peer.notifyPresentPieces(notifyPresentPiecesErrors)

	sendKeepAliveErrors := make(chan error)
	go peer.sendKeepAlive(sendKeepAliveErrors)

	requestBlocksErrors := make(chan error)
	go peer.requestBlocks(torrent, requestBlocksErrors)

	listenErrors := make(chan error)
	go peer.listen(torrent, downloadedPieces, listenErrors)

	uploadPiecesErrors := make(chan error)
	go peer.uploadPieces(downloadedPieces, uploadPiecesErrors)

	go peer.checkStalePieceRequests()

	select {
	case err := <-sendKeepAliveErrors:
		return err
	case err := <-requestBlocksErrors:
		return err
	case err := <-listenErrors:
		return err
	case err := <-notifyPresentPiecesErrors:
		return err
	case err := <-uploadPiecesErrors:
		return err
	}
}

func (peer *Peer) listen(
	torrent *torrent_info.TorrentInfo,
	downloadedPieces *download.Download,
	errors chan<- error,
) {
	for {
		receivedMessage, err := message.Decode(peer.connection)
		if err != nil {
			errors <- fmt.Errorf("failed to decode message: %w", err)
			break
		}

		if receivedMessage == nil {
			// No message is received
			continue
		}

		switch msg := receivedMessage.(type) {
		case *message.KeepAlive:
			continue
		case *message.Choke:
			peer.chocked = true
		case *message.Unchoke:
			peer.chocked = false
		case *message.Interested:
			// TODO
		case *message.NotInterested:
			// TODO
		case *message.Have:
			peer.availablePieces.AddPiece(msg.Piece)
		case *message.Bitfield:
			peer.availablePieces = bitfield.NewConcurrentBitfield(
				msg.Bitfield,
				len(torrent.Pieces),
			)
		case *message.Request:
			request := requested_pieces.PieceRequest{Piece: msg.Piece, Offset: msg.Offset, Length: msg.Length}
			peer.requestedPieces.AddRequest(request)
		case *message.Piece:
			donePiece, err := peer.pendingPieces.InsertData(msg.Piece, msg.Offset, msg.Data)
			if err != nil {
				// TODO: Process this error?.
				log.Printf("failed to insert data to the pending piece: %v", err)
			}

			if donePiece != nil {
				log.Printf("received piece #%d", msg.Piece)

				sha1 := sha1.Sum(donePiece.Data)
				var newState pieces.PieceState
				if torrent.Pieces[msg.Piece] != sha1 {
					log.Printf(
						"received piece with invalid hash: expected %s, got %s",
						hex.EncodeToString(torrent.Pieces[msg.Piece][:]),
						hex.EncodeToString(sha1[:]),
					)

					newState = pieces.NotDownloaded
				} else {
					globalOffset := int(msg.Piece) * torrent.PieceLength
					downloadedPieces.WritePiece(
						download.DownloadedPiece{Offset: globalOffset, Data: donePiece.Data},
					)

					newState = pieces.Downloaded
				}

				if !peer.pieces.CheckStateAndChange(int(msg.Piece), pieces.Pending, newState) {
					log.Panicf(
						"Piece is in unexpected state. Expected %v, got %v",
						pieces.Pending,
						peer.pieces.GetState(int(msg.Piece)),
					)
				}
			}
		case *message.Cancel:
			request := requested_pieces.PieceRequest{Piece: msg.Piece, Offset: msg.Offset, Length: msg.Length}
			peer.requestedPieces.CancelRequest(request)
		case *message.ExtendedHandshake:
			peer.availableExtensions, err = extensions.FromMap(msg.SupportedExtensions)
			if err != nil {
				errors <- fmt.Errorf("failed to decode extensions: %w", err)
			}
			peer.clientName = msg.ClientName
		}
	}
}

func (peer *Peer) requestBlocks(
	torrent *torrent_info.TorrentInfo,
	errors chan<- error,
) {
Outer:
	for {
		if peer.chocked {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		for pieceIdx := range len(torrent.Pieces) {
			if peer.availablePieces == nil || !peer.availablePieces.ContainsPiece(pieceIdx) {
				continue
			}

			if peer.pendingPieces.Length() >= constants.PendingPiecesQueueLength {
				time.Sleep(time.Millisecond * 100)
				continue
			}

			if !peer.pieces.CheckStateAndChange(pieceIdx, pieces.NotDownloaded, pieces.Pending) {
				continue
			}

			log.Printf("requesting piece #%d", pieceIdx)

			pieceLength := min(torrent.PieceLength, torrent.TotalLength-pieceIdx*torrent.PieceLength)
			blockCount := (pieceLength + constants.BlockSize - 1) / constants.BlockSize

			peer.pendingPieces.Insert(pieceIdx, pieceLength)

			for block := range blockCount {
				length := min(constants.BlockSize, pieceLength-block*constants.BlockSize)

				message := message.Request{
					Piece:  pieceIdx,
					Offset: block * constants.BlockSize,
					Length: length,
				}
				request := (&message).Encode()
				_, err := peer.connection.Write(request)
				if err != nil {
					peer.pendingPieces.Remove(pieceIdx)
					errors <- fmt.Errorf("error sending piece request: %w", err)
					break Outer
				}
			}
		}
	}
}

func (peer *Peer) sendInitialMessages() error {
	present := peer.pieces.GetBitfield()
	if !present.IsEmpty() {
		request := (&message.Bitfield{Bitfield: present.ToBytes()}).Encode()
		_, err := peer.connection.Write(request)
		if err != nil {
			return fmt.Errorf("error sending bitfield: %w", err)
		}

		log.Printf("sent bitfield message")
	}

	request := (&message.Interested{}).Encode()
	_, err := peer.connection.Write(request)
	if err != nil {
		return fmt.Errorf("error sending interested message: %w", err)
	}

	log.Printf("sent interested message")

	request = (&message.Unchoke{}).Encode()
	_, err = peer.connection.Write(request)
	if err != nil {
		return fmt.Errorf("error sending unchoke message: %w", err)
	}

	log.Printf("sent unchoke message")

	return nil
}

func (peer *Peer) notifyPresentPieces(errors chan<- error) {
	availability := peer.pieces.GetBitfield()

	for {
		currentAvailability := peer.pieces.GetBitfield()
		newAvailable := currentAvailability.Subtract(&availability)

		if newAvailable.IsEmpty() {
			time.Sleep(constants.NotifyPresentPiecesInterval)
			continue
		}

		availability = currentAvailability

		for piece := range newAvailable.PieceCount() {
			if newAvailable.ContainsPiece(piece) {
				message := message.Have{Piece: piece}
				_, err := peer.connection.Write(message.Encode())
				if err != nil {
					errors <- fmt.Errorf("error sending have message: %w", err)
					break
				}
			}
		}
	}
}

func (peer *Peer) uploadPieces(downloadedPieces *download.Download, errors chan<- error) {
	for {
		requestedPiece := peer.requestedPieces.PopRequest()
		if requestedPiece == nil {
			time.Sleep(constants.RequestedPiecesPopInterval)
			continue
		}

		pieceData, err := downloadedPieces.ReadPiece(requestedPiece.Piece)
		if err != nil {
			errors <- fmt.Errorf("failed to read piece #%d: %w", requestedPiece.Piece, err)
		}

		// TODO: Add method to partially read piece.
		block := (*pieceData)[requestedPiece.Offset : requestedPiece.Offset+requestedPiece.Length]

		message := message.Piece{Piece: requestedPiece.Piece, Offset: requestedPiece.Offset, Data: block}
		_, err = peer.connection.Write(message.Encode())
		if err != nil {
			errors <- fmt.Errorf("error sending piece message: %w", err)
			break
		}
	}
}

func (peer *Peer) checkStalePieceRequests() {
	for {
		time.Sleep(constants.PieceRequestTimeout / 10)

		stalePieces := peer.pendingPieces.RemoveStale()
		for _, stale := range stalePieces {
			peer.pieces.CheckStateAndChange(stale, pieces.Pending, pieces.NotDownloaded)
		}
	}
}

func (peer *Peer) sendKeepAlive(errors chan<- error) {
	for {
		time.Sleep(constants.KeepAliveInterval)

		message := (&message.KeepAlive{}).Encode()
		_, err := peer.connection.Write(message)
		if err != nil {
			errors <- fmt.Errorf("error sending keep-alive message: %w", err)
			break
		}

		log.Printf("sent keep-alive message")
	}
}
