// Package rcon implements the Source RCON protocol Minecraft servers speak
// (enable-rcon=true in server.properties), used both for the free-text
// console (FR-15) and GUI command buttons (FR-17) -- both go through
// Client.Execute, satisfying FR-18's "same execution path" requirement.
package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	packetTypeExecCommand  = 2
	packetTypeAuth         = 3
	packetTypeAuthResponse = 2 // yes, same wire value as ExecCommand's response type
	packetTypeResponseValue = 0
)

type Client struct {
	conn      net.Conn
	nextReqID int32
}

// Dial connects to addr (host:port) and authenticates with password. It
// retries the connection itself for up to timeout, since callers typically
// call this right after starting a Minecraft process, which needs a few
// seconds before its RCON listener is up.
func Dial(addr, password string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		c := &Client{conn: conn, nextReqID: 1}
		if err := c.authenticate(password); err != nil {
			conn.Close()
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return c, nil
	}
	return nil, fmt.Errorf("rcon dial %s: %w", addr, lastErr)
}

func (c *Client) authenticate(password string) error {
	reqID := c.nextReqID
	c.nextReqID++
	if err := c.writePacket(reqID, packetTypeAuth, password); err != nil {
		return err
	}
	// The server may send an empty SERVERDATA_RESPONSE_VALUE before the
	// actual auth response; skip packets until we see the auth response
	// type (or run out of patience).
	for i := 0; i < 2; i++ {
		respID, respType, _, err := c.readPacket()
		if err != nil {
			return fmt.Errorf("read auth response: %w", err)
		}
		if respType != packetTypeAuthResponse {
			continue
		}
		if respID == -1 {
			return fmt.Errorf("rcon authentication rejected (wrong password)")
		}
		return nil
	}
	return fmt.Errorf("rcon authentication: no auth response received")
}

// Execute runs command and returns the server's response text.
func (c *Client) Execute(command string) (string, error) {
	reqID := c.nextReqID
	c.nextReqID++
	if err := c.writePacket(reqID, packetTypeExecCommand, command); err != nil {
		return "", err
	}
	_, _, payload, err := c.readPacket()
	if err != nil {
		return "", fmt.Errorf("read command response: %w", err)
	}
	return payload, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) writePacket(reqID int32, packetType int32, payload string) error {
	body := []byte(payload)
	// size = 4 (reqID) + 4 (type) + len(body) + 1 (payload terminator) + 1 (packet terminator)
	size := int32(4 + 4 + len(body) + 2)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, size)
	binary.Write(buf, binary.LittleEndian, reqID)
	binary.Write(buf, binary.LittleEndian, packetType)
	buf.Write(body)
	buf.Write([]byte{0, 0})

	_, err := c.conn.Write(buf.Bytes())
	return err
}

func (c *Client) readPacket() (reqID int32, packetType int32, payload string, err error) {
	var size int32
	if err := binary.Read(c.conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return 0, 0, "", err
	}
	reqID = int32(binary.LittleEndian.Uint32(body[0:4]))
	packetType = int32(binary.LittleEndian.Uint32(body[4:8]))
	// body[8:len-2] is the payload; the last two bytes are null terminators.
	payload = string(bytes.TrimRight(body[8:len(body)-2], "\x00"))
	return reqID, packetType, payload, nil
}
