// Package mcping implements just enough of Minecraft's Server List Ping
// protocol (the same one the game client uses to show a server's player
// count/sample in the multiplayer menu) to query online players. Unlike
// RCON's "list" command, this is a fixed binary/JSON protocol that plugins
// have no way to reformat -- confirmed on real hardware that EssentialsX
// rewrites "list"'s text output entirely, silently breaking any parser
// built around vanilla's exact wording. Status Ping bypasses that class of
// problem altogether.
package mcping

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

type Player struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Status struct {
	Players struct {
		Max    int      `json:"max"`
		Online int      `json:"online"`
		Sample []Player `json:"sample"`
	} `json:"players"`
}

// Ping performs a Status Ping handshake against a Minecraft server listening
// on addr ("host:port") and returns its reported player count/sample.
func Ping(ctx context.Context, addr string, timeout time.Duration) (*Status, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("parse port %q: %w", portStr, err)
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	// Handshake packet: protocol version (-1 = "don't care", valid for a
	// status-only ping), server address, server port, next state (1 = status).
	handshake := new(bytes.Buffer)
	writeVarInt(handshake, -1)
	writeString(handshake, host)
	binary.Write(handshake, binary.BigEndian, uint16(port))
	writeVarInt(handshake, 1)
	if err := writePacket(conn, 0x00, handshake.Bytes()); err != nil {
		return nil, fmt.Errorf("send handshake: %w", err)
	}

	// Status Request: packet ID 0x00, no payload.
	if err := writePacket(conn, 0x00, nil); err != nil {
		return nil, fmt.Errorf("send status request: %w", err)
	}

	_, data, err := readPacket(conn)
	if err != nil {
		return nil, fmt.Errorf("read status response: %w", err)
	}

	r := bytes.NewReader(data)
	jsonLen, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read status response json length: %w", err)
	}
	jsonBytes := make([]byte, jsonLen)
	if _, err := io.ReadFull(r, jsonBytes); err != nil {
		return nil, fmt.Errorf("read status response json: %w", err)
	}

	var status Status
	if err := json.Unmarshal(jsonBytes, &status); err != nil {
		return nil, fmt.Errorf("parse status response json: %w", err)
	}
	return &status, nil
}

func writePacket(w io.Writer, packetID int32, data []byte) error {
	payload := new(bytes.Buffer)
	writeVarInt(payload, packetID)
	payload.Write(data)

	packet := new(bytes.Buffer)
	writeVarInt(packet, int32(payload.Len()))
	packet.Write(payload.Bytes())

	_, err := w.Write(packet.Bytes())
	return err
}

// readPacket reads one length-prefixed packet and returns its ID and
// remaining payload bytes.
func readPacket(r io.Reader) (packetID int32, data []byte, err error) {
	length, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	br := bytes.NewReader(buf)
	packetID, err = readVarInt(br)
	if err != nil {
		return 0, nil, err
	}
	data = buf[len(buf)-br.Len():]
	return packetID, data, nil
}

func writeString(buf *bytes.Buffer, s string) {
	writeVarInt(buf, int32(len(s)))
	buf.WriteString(s)
}

// writeVarInt encodes value using the protocol's variable-length integer
// format (7 payload bits per byte, high bit set on all but the last byte).
func writeVarInt(buf *bytes.Buffer, value int32) {
	uv := uint32(value)
	for {
		if uv&^0x7F == 0 {
			buf.WriteByte(byte(uv))
			return
		}
		buf.WriteByte(byte(uv&0x7F) | 0x80)
		uv >>= 7
	}
}

func readVarInt(r io.Reader) (int32, error) {
	var result int32
	var numRead uint
	for {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		result |= int32(b[0]&0x7F) << (7 * numRead)
		numRead++
		if numRead > 5 {
			return 0, fmt.Errorf("varint is too long")
		}
		if b[0]&0x80 == 0 {
			break
		}
	}
	return result, nil
}
