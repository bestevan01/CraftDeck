package ddns

import (
	"context"
	"fmt"
	"net"
)

// CheckMismatch resolves hostname's current A/AAAA records and checks
// whether currentPublicIP is among them -- FR-26f's drift check for a
// monitor-only provider (ipTime today) that CraftDeck has no API to
// actively renew. A hostname can legitimately have more than one address
// (round-robin DNS), so this only flags a mismatch when currentPublicIP
// isn't present in the answer at all, rather than comparing against
// whichever address happens to come back first. resolvedIP (for display)
// is always the first answer.
func CheckMismatch(ctx context.Context, hostname, currentPublicIP string) (resolvedIP string, mismatch bool, err error) {
	ips, err := net.DefaultResolver.LookupHost(ctx, hostname)
	if err != nil {
		return "", false, fmt.Errorf("resolve %s: %w", hostname, err)
	}
	if len(ips) == 0 {
		return "", false, fmt.Errorf("no addresses found for %s", hostname)
	}
	resolvedIP = ips[0]
	for _, ip := range ips {
		if ip == currentPublicIP {
			return resolvedIP, false, nil
		}
	}
	return resolvedIP, true, nil
}
