package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Minimal WebSocket endpoint (/socket) used by Jellyfin clients for keep-alive
// messages. Remote control is not supported, so only keep-alives are handled.

func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h[:])
}

func wsWrite(w io.Writer, mu *sync.Mutex, opcode byte, payload []byte) error {
	mu.Lock()
	defer mu.Unlock()
	header := []byte{0x80 | opcode}
	switch n := len(payload); {
	case n < 126:
		header = append(header, byte(n))
	case n < 65536:
		header = append(header, 126, byte(n>>8), byte(n))
	default:
		header = append(header, 127)
		header = binary.BigEndian.AppendUint64(header, uint64(n))
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func wsRead(r *bufio.Reader) (byte, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	opcode := h[0] & 0x0f
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > 1<<20 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	var mask [4]byte
	if h[1]&0x80 != 0 {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, err
	}
	if h[1]&0x80 != 0 {
		for i := range data {
			data[i] ^= mask[i%4]
		}
	}
	return opcode, data, nil
}

func (s *Server) jfSocket(w http.ResponseWriter, r *http.Request) error {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return errf(http.StatusBadRequest, "websocket required")
	}
	if _, err := s.userForToken(parseJFAuth(r).Token); err != nil {
		return errf(http.StatusUnauthorized, "Unauthorized")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return errf(http.StatusInternalServerError, "hijack unsupported")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		wsAccept(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n")
	if err := rw.Flush(); err != nil {
		return nil
	}
	var mu sync.Mutex
	send := func(v any) error {
		b, _ := json.Marshal(v)
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return wsWrite(conn, &mu, 1, b)
	}
	_ = send(map[string]any{"MessageType": "ForceKeepAlive", "Data": 60})
	for {
		_ = conn.SetReadDeadline(time.Now().Add(150 * time.Second))
		op, data, err := wsRead(rw.Reader)
		if err != nil {
			return nil
		}
		switch op {
		case 8: // close
			_ = wsWrite(conn, &mu, 8, nil)
			return nil
		case 9: // ping
			_ = wsWrite(conn, &mu, 10, data)
		case 1:
			var msg struct{ MessageType string }
			if json.Unmarshal(data, &msg) == nil && msg.MessageType == "KeepAlive" {
				_ = send(map[string]any{"MessageType": "KeepAlive"})
			}
		}
	}
}
