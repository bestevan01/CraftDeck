package ddns

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// duckDNSUpdateURLFmt implements DuckDNS's own update API
// (https://www.duckdns.org/spec.jsp) -- a plain authenticated GET that
// returns the literal text "OK" or "KO". ipv6 is a separate query param
// DuckDNS accepts alongside ip (FR-30's AAAA requirement) -- omitted
// entirely (rather than sent empty) when there's no public IPv6 to report,
// since DuckDNS treats an explicit empty value as "clear the AAAA record".
const duckDNSUpdateURLFmt = "https://www.duckdns.org/update?domains=%s&token=%s&ip=%s"

type duckDNSUpdater struct{}

func (duckDNSUpdater) Update(ctx context.Context, hostname, token, ipv4, ipv6 string) error {
	// DuckDNS's API takes just the subdomain label (e.g. "myserver"), not
	// the full "myserver.duckdns.org" hostname CraftDeck stores/displays.
	label := strings.TrimSuffix(hostname, ".duckdns.org")

	reqURL := fmt.Sprintf(duckDNSUpdateURLFmt, url.QueryEscape(label), url.QueryEscape(token), url.QueryEscape(ipv4))
	if ipv6 != "" {
		reqURL += "&ipv6=" + url.QueryEscape(ipv6)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("duckdns update request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return fmt.Errorf("read duckdns response: %w", err)
	}
	if result := strings.TrimSpace(string(body)); result != "OK" {
		return fmt.Errorf("duckdns rejected the update (check the domain/token): %q", result)
	}
	return nil
}
