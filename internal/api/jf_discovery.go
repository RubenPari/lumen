package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// ServeDiscovery answers Jellyfin client auto-discovery broadcasts on UDP 7359
// ("who is JellyfinServer?") so apps like Jellyfin for Android TV find Lumen.
func (s *Server) ServeDiscovery(ctx context.Context, httpPort string) {
	conn, err := net.ListenPacket("udp4", ":7359")
	if err != nil {
		s.Log.Warn("discovery disabled", "err", err)
		return
	}
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	buf := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		msg := strings.ToLower(strings.TrimSpace(string(buf[:n])))
		if !strings.Contains(msg, "who is jellyfinserver") && !strings.Contains(msg, "who is embyserver") {
			continue
		}
		host, _ := os.Hostname()
		local := localIPFor(addr)
		resp, _ := json.Marshal(map[string]any{
			"Address":         fmt.Sprintf("http://%s:%s", local, httpPort),
			"Id":              s.serverID(),
			"Name":            s.DB.GetSetting("server_name", host),
			"EndpointAddress": nil,
		})
		_, _ = conn.WriteTo(resp, addr)
	}
}

// localIPFor returns the local address used to reach the client.
func localIPFor(addr net.Addr) string {
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ua.IP, Port: 9})
	if err != nil {
		return "127.0.0.1"
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}
