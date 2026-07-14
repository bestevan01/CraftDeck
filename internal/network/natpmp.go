package network

import (
	"fmt"
	"net"
	"strings"
	"time"

	natpmp "github.com/jackpal/go-nat-pmp"
)

// natPMPLifetimeSeconds is the requested mapping lifetime for a NAT-PMP
// AddPortMapping call. Unlike UPnP (where leaseDuration=0 means "forever"),
// NAT-PMP defines a lifetime of 0 as a request to *delete* the mapping
// (RFC 6886 section 3.3) -- so a mapping meant to stay up needs a real,
// finite lifetime and periodic renewal well before it expires (see
// natPMPRenewInterval in manager.go).
const natPMPLifetimeSeconds = 3600 // 1 hour

func addViaNATPMP(gateway net.IP, externalPort, internalPort int, protocol string) error {
	if gateway == nil {
		return fmt.Errorf("no gateway IP available for NAT-PMP")
	}
	client := natpmp.NewClient(gateway)
	_, err := client.AddPortMapping(strings.ToLower(protocol), internalPort, externalPort, natPMPLifetimeSeconds)
	if err != nil {
		return fmt.Errorf("nat-pmp AddPortMapping: %w", err)
	}
	return nil
}

func deleteViaNATPMP(gateway net.IP, internalPort int, protocol string) error {
	if gateway == nil {
		return fmt.Errorf("no gateway IP available for NAT-PMP")
	}
	client := natpmp.NewClient(gateway)
	// A lifetime of 0 is NAT-PMP's own "delete this mapping" signal (RFC
	// 6886 3.3), not a real port request -- requestedExternalPort is
	// ignored by the protocol in a delete, but the library still wants a
	// value so 0 is passed.
	_, err := client.AddPortMapping(strings.ToLower(protocol), internalPort, 0, 0)
	if err != nil {
		return fmt.Errorf("nat-pmp delete (zero-lifetime AddPortMapping): %w", err)
	}
	return nil
}

// natPMPRenewInterval re-applies a NAT-PMP mapping well before its
// natPMPLifetimeSeconds expiry, so it survives indefinitely as long as the
// router keeps responding.
const natPMPRenewInterval = 45 * time.Minute
