package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// A2SInfo holds the parsed response from an A2S_INFO query.
type A2SInfo struct {
	Name       string
	Map        string
	Players    uint8
	MaxPlayers uint8
}

// a2sInfoRequest is the standard A2S_INFO request packet.
// Header (4 bytes 0xFF) + type byte 'T' + payload "Source Engine Query\x00"
var a2sInfoRequest = []byte{
	0xFF, 0xFF, 0xFF, 0xFF, 0x54,
	0x53, 0x6F, 0x75, 0x72, 0x63, 0x65, 0x20,
	0x45, 0x6E, 0x67, 0x69, 0x6E, 0x65, 0x20,
	0x51, 0x75, 0x65, 0x72, 0x79, 0x00,
}

// QueryA2SInfo sends an A2S_INFO UDP query to the given address and returns
// parsed server info. Returns an error if the server doesn't respond or the
// response is malformed.
func QueryA2SInfo(addr string, timeout time.Duration) (*A2SInfo, error) {
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write(a2sInfoRequest); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	buf := make([]byte, 1400)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return parseA2SInfo(buf[:n])
}

// parseA2SInfo parses a raw A2S_INFO response packet.
// Format: FF FF FF FF 49 <protocol> <name\0> <map\0> <folder\0> <game\0> <appid:2> <players:1> <maxplayers:1> ...
func parseA2SInfo(data []byte) (*A2SInfo, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}

	// Verify header
	if !bytes.Equal(data[:4], []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
		return nil, fmt.Errorf("invalid header")
	}

	// Check for challenge response (0x41 = 'A')
	if data[4] == 0x41 {
		return nil, fmt.Errorf("server sent challenge; not supported")
	}

	// Type byte should be 0x49 ('I') for A2S_INFO response
	if data[4] != 0x49 {
		return nil, fmt.Errorf("unexpected response type: 0x%02x", data[4])
	}

	r := bytes.NewReader(data[5:])
	info := &A2SInfo{}

	// Protocol version (1 byte)
	var protocol uint8
	if err := binary.Read(r, binary.LittleEndian, &protocol); err != nil {
		return nil, fmt.Errorf("read protocol: %w", err)
	}

	// Server name (null-terminated string)
	name, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("read name: %w", err)
	}
	info.Name = name

	// Map (null-terminated string)
	mapName, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("read map: %w", err)
	}
	info.Map = mapName

	// Folder (null-terminated string) - skip
	if _, err := readString(r); err != nil {
		return nil, fmt.Errorf("read folder: %w", err)
	}

	// Game name (null-terminated string) - skip
	if _, err := readString(r); err != nil {
		return nil, fmt.Errorf("read game: %w", err)
	}

	// App ID (2 bytes) - skip
	var appID uint16
	if err := binary.Read(r, binary.LittleEndian, &appID); err != nil {
		return nil, fmt.Errorf("read appid: %w", err)
	}

	// Players (1 byte)
	if err := binary.Read(r, binary.LittleEndian, &info.Players); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}

	// Max players (1 byte)
	if err := binary.Read(r, binary.LittleEndian, &info.MaxPlayers); err != nil {
		return nil, fmt.Errorf("read maxplayers: %w", err)
	}

	return info, nil
}

// readString reads a null-terminated string from a bytes.Reader.
func readString(r *bytes.Reader) (string, error) {
	var result []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == 0 {
			return string(result), nil
		}
		result = append(result, b)
	}
}
