package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
)

// upnpConnection is the AddPortMapping/DeletePortMapping shape shared by
// every IGD WANIPConnection client goupnp generates (v1 and v2) -- letting
// addViaUPnP/deleteViaUPnP try whichever version(s) respond without caring
// which one actually implements it.
type upnpConnection interface {
	AddPortMapping(remoteHost string, externalPort uint16, protocol string, internalPort uint16, internalClient string, enabled bool, description string, leaseDuration uint32) error
	DeletePortMapping(remoteHost string, externalPort uint16, protocol string) error
}

// discoverUPnPConnections searches the LAN (SSDP multicast) for every IGD
// WANIPConnection service it can reach, preferring IGDv2 (internetgateway2)
// since it's the current spec, but also collecting IGDv1
// (internetgateway1) responders since plenty of consumer routers still
// only implement the older version.
func discoverUPnPConnections(ctx context.Context) ([]upnpConnection, error) {
	var conns []upnpConnection

	if v2, _, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx); err == nil {
		for _, c := range v2 {
			conns = append(conns, c)
		}
	}
	if v1, _, err := internetgateway1.NewWANIPConnection1ClientsCtx(ctx); err == nil {
		for _, c := range v1 {
			conns = append(conns, c)
		}
	}
	if len(conns) == 0 {
		return nil, fmt.Errorf("no UPnP IGD (WANIPConnection) found on the network")
	}
	return conns, nil
}

// addViaUPnP asks every discovered IGD to map externalPort -> internalPort
// on internalClient (this host's LAN IP), stopping at the first one that
// accepts it. leaseDuration=0 means "no expiration" per the IGD spec (the
// opposite convention from NAT-PMP -- see addViaNATPMP), so no renewal
// loop is needed for UPnP-sourced mappings.
func addViaUPnP(ctx context.Context, externalPort, internalPort int, protocol, internalClient, description string) error {
	conns, err := discoverUPnPConnections(ctx)
	if err != nil {
		return err
	}
	proto := strings.ToUpper(protocol)
	var lastErr error
	for _, c := range conns {
		if err := c.AddPortMapping("", uint16(externalPort), proto, uint16(internalPort), internalClient, true, description, 0); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("upnp AddPortMapping rejected by every discovered IGD: %w", lastErr)
}

func deleteViaUPnP(ctx context.Context, externalPort int, protocol string) error {
	conns, err := discoverUPnPConnections(ctx)
	if err != nil {
		return err
	}
	proto := strings.ToUpper(protocol)
	var lastErr error
	for _, c := range conns {
		if err := c.DeletePortMapping("", uint16(externalPort), proto); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("upnp DeletePortMapping rejected by every discovered IGD: %w", lastErr)
}
