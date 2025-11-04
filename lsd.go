package torrent

import (
	"io"
	"net"
	"net/netip"

	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/trim21/bep14"
)

type LsdServer interface {
	Stats() interface{}
	Start()
	Announce(hash string) error
	WriteStatus(io.Writer)
	Close() error
}

type LsdAnnounce struct {
	InfoHashes []string
	Source     netip.AddrPort
}

type LsdServerWrapper struct {
	*bep14.LSP
	logger log.Logger
	cl     *Client
}

func (me LsdServerWrapper) Stats() interface{} {
	// LSD doesn't have stats like DHT, return a simple placeholder
	return map[string]interface{}{
		"status": "running",
	}
}

func (me LsdServerWrapper) Start() {
	go me.LSP.Start()
	go me.consumeAnnouncements()
}

func (me LsdServerWrapper) Announce(hash string) error {
	return me.LSP.Announce([]string{hash})
}

func (me LsdServerWrapper) WriteStatus(w io.Writer) {
	// Write basic LSD status
	w.Write([]byte("LSD Server Status: Running\n"))
}

func (me LsdServerWrapper) Close() error {

	if me.LSP != nil {
		return me.LSP.Close()
	}
	return nil
}

func (me LsdServerWrapper) consumeAnnouncements() {
	for announce := range me.LSP.C {
		me.handleAnnounce(announce)
	}
}

func (me LsdServerWrapper) handleAnnounce(announce bep14.Announce) {
	// Convert the announcement to peers and add them to torrents
	for _, infoHash := range announce.InfoHashes {

		if me.cl.dopplegangerAddr(announce.Source.String()) {
			continue
		}

		var infoHashBytes metainfo.Hash
		err := infoHashBytes.FromHexString(infoHash)
		if err != nil {
			me.logger.Log(log.Fstr("Failed to decode infohash %s: %v", infoHash, err))
			continue
		}
		// Add peer to the appropriate torrent
		me.addPeerFromLsdAnnounce(infoHashBytes, announce.Source)
	}
}

func (me LsdServerWrapper) addPeerFromLsdAnnounce(infoHash metainfo.Hash, source netip.AddrPort) {
	// Lock the client

	me.cl.lock()
	defer me.cl.unlock()

	// Find the torrent
	t := me.cl.torrent(infoHash)
	if t == nil {
		// Torrent not found, ignore
		return
	}

	// Convert netip.AddrPort to IP and port
	ip := net.IP(source.Addr().AsSlice())
	port := int(source.Port())

	// Add peer to the torrent

	t.addPeers([]Peer{{
		IP:     ip,
		Port:   port,
		Source: peerSourceLsd,
	}})
}
