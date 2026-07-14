package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// publicIPServiceURL is a plain-text "what is my IP" echo service. Asking
// this instead of the router directly (UPnP/NAT-PMP both expose a
// GetExternalIPAddress-style call) means it works regardless of how -- or
// whether -- port forwarding was actually set up (including the FR-23
// manual fallback, where there's no router API call to make at all), and
// reflects what the public internet genuinely sees this connection as
// rather than whatever the router reports on its WAN interface (not
// necessarily the same thing behind multiple layers of NAT).
const publicIPServiceURL = "https://api.ipify.org"

// publicIPv6ServiceURL is ipify's IPv6-only endpoint (as opposed to
// publicIPServiceURL, which is IPv4-only, and api64.ipify.org, which is
// whichever protocol the requesting host happens to prefer) -- used for
// FR-28/30's AAAA record automation. Most home connections have no public
// IPv6 at all, so failing to reach this is an expected, not-necessarily-
// worth-logging outcome -- see FetchPublicIPv6's callers, which all treat
// its error as "skip AAAA" rather than a hard failure.
const publicIPv6ServiceURL = "https://api6.ipify.org"

// FetchPublicIP asks publicIPServiceURL for this host's current public IPv4
// address.
func FetchPublicIP(ctx context.Context) (string, error) {
	return fetchIP(ctx, publicIPServiceURL)
}

// FetchPublicIPv6 asks publicIPv6ServiceURL for this host's current public
// IPv6 address. Returns an error whenever the network genuinely has no
// public IPv6 connectivity (still the common case for home routers) --
// callers should treat that as "no AAAA record to manage" rather than
// surfacing it as a failure.
func FetchPublicIPv6(ctx context.Context) (string, error) {
	return fetchIP(ctx, publicIPv6ServiceURL)
}

func fetchIP(ctx context.Context, serviceURL string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serviceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch public ip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, serviceURL)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// LocalIP returns this host's own LAN IP address (the interface that
// reaches the default gateway) -- what a player on the same home network
// should connect to instead of the public IP.
func LocalIP(ctx context.Context) (string, error) {
	route, err := defaultRoute(ctx)
	if err != nil {
		return "", err
	}
	return route.localIP.String(), nil
}
