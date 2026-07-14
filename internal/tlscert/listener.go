package tlscert

import (
	"bufio"
	"crypto/tls"
	"net"
	"time"
)

// ConditionalListener wraps a raw net.Listener so the same http.Server can
// serve both plain HTTP and real TLS on the same port without ever
// restarting the listener -- each newly accepted connection is sniffed for
// a TLS ClientHello and wrapped accordingly.
//
// This has to be content-sniffed per connection rather than gated on the
// WAN-exposure toggle (FR-21/25): confirmed on real hardware that gating on
// the toggle instead breaks the operator's own reverse proxy (NPM) sitting
// in front of this port on the LAN, which connects with plain HTTP
// (forward_scheme=http) regardless of whether WAN exposure happens to be
// on -- the toggle describes whether the port is forwarded to the internet,
// not what protocol any given caller (an NPM backend connection, a LAN
// browser, or an actual WAN client) happens to be speaking to it.
type ConditionalListener struct {
	net.Listener
	TLSConfig *tls.Config
}

// peekTimeout bounds how long Accept waits to see the first byte before
// deciding plain-vs-TLS -- generous for a real client (which sends its
// first byte immediately), short enough that a connection opened but never
// written to doesn't tie up the accept loop indefinitely.
const peekTimeout = 5 * time.Second

func (l *ConditionalListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(peekTimeout))
	br := bufio.NewReader(conn)
	first, err := br.Peek(1)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, err
	}

	pc := &peekedConn{Conn: conn, r: br}
	// 0x16 is the TLS record type for a Handshake message -- every real TLS
	// ClientHello starts with it; plain HTTP always starts with an ASCII
	// method letter instead.
	if first[0] == 0x16 {
		return tls.Server(pc, l.TLSConfig), nil
	}
	return pc, nil
}

// peekedConn re-plays the byte(s) already consumed from conn by Accept's
// bufio.Peek before handing the connection off, so nothing is lost from
// the caller's perspective.
type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
