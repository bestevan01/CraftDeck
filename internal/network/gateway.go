package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// routeInfo is the default-route gateway and this host's own LAN IP on the
// interface that reaches it -- both come from the same `ip route get`
// query, which is why they're fetched together rather than via two
// separate cross-platform-routing-table libraries. Shelling out to `ip`
// (already present on any Debian/Raspberry Pi OS install, unlike adding a
// Go routing-table dependency) matches how the rest of the daemon already
// prefers a Linux-native CLI tool over a new library for OS-level queries
// (systemd-run/systemctl/journalctl in internal/process).
type routeInfo struct {
	gateway net.IP
	localIP net.IP
}

// defaultRoute finds the gateway and local source IP for the interface
// that would carry traffic to the public internet -- 1.1.1.1 is just a
// routing-table probe target here, never actually contacted.
func defaultRoute(ctx context.Context) (*routeInfo, error) {
	out, err := exec.CommandContext(ctx, "ip", "route", "get", "1.1.1.1").Output()
	if err != nil {
		return nil, fmt.Errorf("determine default route: %w", err)
	}
	// Typical output: "1.1.1.1 via 192.168.0.1 dev eth0 src 192.168.0.5 uid 0"
	fields := strings.Fields(string(out))
	info := &routeInfo{}
	for i, f := range fields {
		switch f {
		case "via":
			if i+1 < len(fields) {
				info.gateway = net.ParseIP(fields[i+1])
			}
		case "src":
			if i+1 < len(fields) {
				info.localIP = net.ParseIP(fields[i+1])
			}
		}
	}
	if info.localIP == nil {
		return nil, fmt.Errorf("could not parse local IP from `ip route get` output: %q", strings.TrimSpace(string(out)))
	}
	return info, nil
}
