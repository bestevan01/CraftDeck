// Package dns implements requirements.md's FR-28~31 owned-main-domain
// automation via the Cloudflare API -- the DNS provider CraftDeck targets
// first (see requirements.md's DDNS comparison table), since it offers a
// free API-token-scoped-to-one-zone workflow that's already the common way
// to manage a domain bought elsewhere (change nameservers, keep the
// registrar). Other providers would need their own adapter, mirroring how
// internal/ddns's free-subdomain providers are structured.
package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const apiBase = "https://api.cloudflare.com/client/v4"

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareZonesResponse struct {
	Success bool             `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  []cloudflareZone `json:"result"`
}

type cloudflareError struct {
	Message string `json:"message"`
}

// VerifyZoneAccess implements FR-31's ownership check: rather than making
// the operator prove domain ownership via a manually-confirmed TXT record
// (the decision explicitly rejected in favor of this -- see the domain
// registration UI's token field), it just asks Cloudflare "does this exact
// API token have access to a zone named domain?". A token scoped to one
// zone (the "Edit zone DNS" template restricted to that zone, as instructed
// in the domain-registration UI) can only answer yes if the operator
// actually controls that zone in their own Cloudflare account, which is
// what ownership verification is actually trying to establish -- no
// separate DNS record round-trip needed. Returns the zone's ID (needed by
// every later record-management call, FR-28/29/30) on success.
func VerifyZoneAccess(ctx context.Context, apiToken, domain string) (zoneID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/zones?name="+domain, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("contact Cloudflare API: %w", err)
	}
	defer resp.Body.Close()

	var parsed cloudflareZonesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("parse Cloudflare API response (status %d): %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return "", fmt.Errorf("cloudflare API rejected the token: %s", parsed.Errors[0].Message)
		}
		return "", fmt.Errorf("cloudflare API rejected the token (status %d)", resp.StatusCode)
	}
	if len(parsed.Result) == 0 {
		return "", fmt.Errorf("this token has no access to a Cloudflare zone named %q -- check the zone name and the token's Zone Resources scope", domain)
	}
	return parsed.Result[0].ID, nil
}

// dnsRecord covers both flat record types (A: Name+Content) and SRV
// (Cloudflare instead wants an SRV record's fields nested under "data",
// with the top-level Name/Content omitted entirely).
type dnsRecord struct {
	ID      string   `json:"id,omitempty"`
	Type    string   `json:"type"`
	Name    string   `json:"name,omitempty"`
	Content string   `json:"content,omitempty"`
	TTL     int      `json:"ttl"`
	Proxied bool     `json:"proxied,omitempty"`
	SRVData *srvData `json:"data,omitempty"`
}

// srvData is deliberately just these four fields -- Cloudflare's own API
// schema puts service/proto/target-domain entirely in the record's
// top-level "name" (the complete "_service._proto.name" string), not
// inside "data" (confirmed against Cloudflare's API docs after data.service/
// proto/name caused "DNS name is invalid" on real hardware).
type srvData struct {
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Port     int    `json:"port"`
	Target   string `json:"target"`
}

type dnsRecordsResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  []dnsRecord       `json:"result"`
}

type dnsRecordResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  dnsRecord         `json:"result"`
}

// UpsertARecord implements FR-28's "create the subdomain's A record" and
// (called again on every WAN IP change, FR-30) keeps it pointed at the
// router's current public IPv4 address -- creates fqdn's A record if it
// doesn't exist yet, or patches its content in place if it does, so a
// forced-host subdomain (see handlers_proxy.go's setServerSubdomain)
// actually resolves to something the moment it's assigned, without the
// operator having to go into Cloudflare and do it by hand.
func UpsertARecord(ctx context.Context, apiToken, zoneID, fqdn, ipv4 string) error {
	return upsertAddressRecord(ctx, apiToken, zoneID, "A", fqdn, ipv4)
}

// UpsertAAAARecord is UpsertARecord's IPv6 counterpart (FR-28/30's AAAA
// requirement) -- callers should only call this when a public IPv6 address
// was actually found (see network.FetchPublicIPv6's doc comment); there's
// no "clear the AAAA record" path here since a connection that had IPv6
// and lost it is treated the same as never having had it; the AAAA record
// just goes stale until IPv6 comes back, same as an A record would if
// FetchPublicIP started failing.
func UpsertAAAARecord(ctx context.Context, apiToken, zoneID, fqdn, ipv6 string) error {
	return upsertAddressRecord(ctx, apiToken, zoneID, "AAAA", fqdn, ipv6)
}

// proxied=false (grey-clouded/DNS-only) because a Minecraft connection is a
// raw TCP stream Cloudflare's proxy can't forward -- only HTTP(S) traffic
// goes through the orange-cloud CDN.
func upsertAddressRecord(ctx context.Context, apiToken, zoneID, recordType, fqdn, ip string) error {
	existingID, err := findRecordID(ctx, apiToken, zoneID, recordType, fqdn)
	if err != nil {
		return err
	}
	record := dnsRecord{Type: recordType, Name: fqdn, Content: ip, TTL: 300, Proxied: false}
	return upsertRecord(ctx, apiToken, zoneID, existingID, record, fqdn)
}

// UpsertSRVRecord implements FR-29: the Minecraft client only skips typing
// a port suffix if `_minecraft._tcp.<fqdn>` resolves to an SRV record
// pointing at wherever the connection is actually served -- fqdn itself,
// since Velocity binds 0.0.0.0 on the proxy's own port and every
// forced-host subdomain routes through that one singleton proxy (FR-1c),
// not a per-server port. So target is always fqdn and port is always the
// proxy's own game_port (see handlers_proxy.go's SyncMainDomainDNS), even
// though requirements.md originally phrased this as "the server instance's
// own forwarding port" -- that phrasing predates forced-host routing
// actually being designed around one shared proxy port rather than
// independent per-server ports.
//
func UpsertSRVRecord(ctx context.Context, apiToken, zoneID, fqdn string, port int) error {
	recordName := "_minecraft._tcp." + fqdn
	existingID, err := findRecordID(ctx, apiToken, zoneID, "SRV", recordName)
	if err != nil {
		return err
	}
	record := dnsRecord{
		Type: "SRV",
		Name: recordName,
		TTL:  300,
		SRVData: &srvData{
			Priority: 0, Weight: 5, Port: port, Target: fqdn,
		},
	}
	return upsertRecord(ctx, apiToken, zoneID, existingID, record, recordName)
}

// DeleteARecord removes fqdn's A record, if one exists -- the counterpart
// to UpsertARecord, called when a server stops being forced-hosted (see
// handlers_proxy.go's removeServerFromProxy) so Cloudflare doesn't keep
// pointing an abandoned subdomain at this server's old IP.
func DeleteARecord(ctx context.Context, apiToken, zoneID, fqdn string) error {
	return deleteRecord(ctx, apiToken, zoneID, "A", fqdn)
}

// DeleteAAAARecord is DeleteARecord's IPv6 counterpart.
func DeleteAAAARecord(ctx context.Context, apiToken, zoneID, fqdn string) error {
	return deleteRecord(ctx, apiToken, zoneID, "AAAA", fqdn)
}

// DeleteSRVRecord removes fqdn's `_minecraft._tcp.<fqdn>` SRV record, if one
// exists -- the counterpart to UpsertSRVRecord.
func DeleteSRVRecord(ctx context.Context, apiToken, zoneID, fqdn string) error {
	return deleteRecord(ctx, apiToken, zoneID, "SRV", "_minecraft._tcp."+fqdn)
}

// deleteRecord looks up name's record ID and, if found, deletes it. A
// no-op (not an error) if no such record exists -- callers may call this
// for a subdomain whose records were never actually created (e.g. the
// Cloudflare sync never ran before it was unassigned).
func deleteRecord(ctx context.Context, apiToken, zoneID, recordType, name string) error {
	existingID, err := findRecordID(ctx, apiToken, zoneID, recordType, name)
	if err != nil {
		return err
	}
	if existingID == "" {
		return nil
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, zoneID, existingID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("contact Cloudflare API: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Success bool              `json:"success"`
		Errors  []cloudflareError `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("parse Cloudflare API response (status %d): %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare API rejected deleting the %s record for %s: %s", recordType, name, parsed.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare API rejected deleting the %s record for %s (status %d)", recordType, name, resp.StatusCode)
	}
	return nil
}

// upsertRecord POSTs record as new if existingID is empty, or PUTs it in
// place (full replace -- Cloudflare's PUT requires the complete record
// definition, not a partial patch) if a matching record was already found.
func upsertRecord(ctx context.Context, apiToken, zoneID, existingID string, record dnsRecord, describeAs string) error {
	method, url := http.MethodPost, fmt.Sprintf("%s/zones/%s/dns_records", apiBase, zoneID)
	if existingID != "" {
		method, url = http.MethodPut, fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, zoneID, existingID)
	}

	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("contact Cloudflare API: %w", err)
	}
	defer resp.Body.Close()

	var parsed dnsRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("parse Cloudflare API response (status %d): %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return fmt.Errorf("cloudflare API rejected the %s record for %s: %s", record.Type, describeAs, parsed.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare API rejected the %s record for %s (status %d)", record.Type, describeAs, resp.StatusCode)
	}
	return nil
}

// findRecordID looks up an existing record's ID by exact type+name, if
// any, so an upsert knows whether to PUT (update) or POST (create).
func findRecordID(ctx context.Context, apiToken, zoneID, recordType, name string) (recordID string, err error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=%s&name=%s", apiBase, zoneID, recordType, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("contact Cloudflare API: %w", err)
	}
	defer resp.Body.Close()

	var parsed dnsRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("parse Cloudflare API response (status %d): %w", resp.StatusCode, err)
	}
	if !parsed.Success {
		if len(parsed.Errors) > 0 {
			return "", fmt.Errorf("cloudflare API rejected the DNS record lookup for %s: %s", name, parsed.Errors[0].Message)
		}
		return "", fmt.Errorf("cloudflare API rejected the DNS record lookup for %s (status %d)", name, resp.StatusCode)
	}
	if len(parsed.Result) == 0 {
		return "", nil
	}
	return parsed.Result[0].ID, nil
}
