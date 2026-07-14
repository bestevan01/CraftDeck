package ddns

import "context"

// Updater is the common adapter interface FR-26c calls for: a free-
// subdomain provider CraftDeck can actively push IP changes to. Adding a
// future provider (FreeDNS, Dynu, No-IP, ...) means implementing this and
// adding one line to updaters below -- no other code changes (FR-26d: each
// adapter is independent, so one provider's API breaking doesn't affect
// the others).
type Updater interface {
	// Update pushes ipv4 as hostname's new A record using token (already
	// decrypted). ipv6 sets/updates the AAAA record too (FR-30) when
	// non-empty -- most home connections have no public IPv6 at all, so
	// callers pass "" whenever none was found rather than treating it as an
	// error (see network.FetchPublicIPv6's doc comment).
	Update(ctx context.Context, hostname, token, ipv4, ipv6 string) error
}

// updaters holds every free-subdomain provider CraftDeck can actively
// renew. FR-26a's implementation order: DuckDNS first, then ipTime -- but
// ipTime has no active-renewal API at all (FR-26b), so it's never an
// Updater; see monitorOnlyProviders instead.
var updaters = map[string]Updater{
	"duckdns": duckDNSUpdater{},
}

// GetUpdater returns provider's Updater, if it has one.
func GetUpdater(provider string) (Updater, bool) {
	u, ok := updaters[provider]
	return u, ok
}

// monitorOnlyProviders lists free-subdomain providers with no third-party
// renewal API (FR-26b/e) -- CraftDeck only watches their hostname for
// drift against the current WAN IP (FR-26f) instead of writing to it.
var monitorOnlyProviders = map[string]bool{
	"iptime": true,
}

// IsMonitorOnly reports whether provider is watch-only rather than
// actively renewable.
func IsMonitorOnly(provider string) bool {
	return monitorOnlyProviders[provider]
}

// SupportedFreeProviders lists every provider name handleSetDomainSettings
// accepts for kind=free_subdomain -- the union of updaters and
// monitorOnlyProviders (FR-26a: DuckDNS and ipTime supported so far;
// FR-26g covers how new provider requests get triaged).
var SupportedFreeProviders = []string{"duckdns", "iptime"}

// IsSupportedFreeProvider reports whether provider is one
// handleSetDomainSettings will accept for kind=free_subdomain.
func IsSupportedFreeProvider(provider string) bool {
	_, updatable := updaters[provider]
	return updatable || monitorOnlyProviders[provider]
}
