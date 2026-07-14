package network

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ManualInfo is what FR-23 shows the operator when neither UPnP nor NAT-PMP
// could set up the mapping automatically -- everything they'd need to type
// into their router's port-forwarding page by hand.
type ManualInfo struct {
	LocalIP      string `json:"local_ip"`
	InternalPort int    `json:"internal_port"`
	ExternalPort int    `json:"external_port"`
	Protocol     string `json:"protocol"`
}

// Manager orchestrates FR-22's "try UPnP, then NAT-PMP" automatic
// port-forwarding, persists what it set up (FR-24, via MappingRepository),
// and keeps NAT-PMP-sourced mappings renewed in the background (see
// natPMPRenewInterval) since that protocol's mappings expire on their own.
// One toggle now covers the web UI port plus every directly-reachable
// Minecraft instance's game port (see internal/api's ReconcileGamePorts),
// so several mappings can be live -- and need independent renewal loops --
// at the same time.
type Manager struct {
	mappings *MappingRepository

	mu       sync.Mutex
	renewals map[string]context.CancelFunc // keyed by PortMapping.ID
}

func NewManager(mappings *MappingRepository) *Manager {
	return &Manager{mappings: mappings, renewals: map[string]context.CancelFunc{}}
}

// Ensure sets up a port mapping for externalPort/internalPort (same value
// for both, kept simple) via UPnP first, then NAT-PMP, persisting a
// PortMapping row on success. It returns a non-nil *ManualInfo (and a nil
// error) instead of failing outright when both automatic methods fail,
// since FR-23 treats "the operator must set it up by hand" as an expected,
// displayable outcome, not an error state.
func (m *Manager) Ensure(ctx context.Context, instanceID *string, port int, protocol, description string) (*PortMapping, *ManualInfo, error) {
	route, err := defaultRoute(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("determine local network info: %w", err)
	}

	if err := addViaUPnP(ctx, port, port, protocol, route.localIP.String(), description); err == nil {
		mapping := &PortMapping{InstanceID: instanceID, ExternalPort: port, InternalPort: port, Protocol: protocol, Method: "upnp"}
		if err := m.mappings.Create(ctx, mapping); err != nil {
			return nil, nil, err
		}
		return mapping, nil, nil
	} else {
		log.Printf("network: upnp port mapping failed, trying nat-pmp: %v", err)
	}

	if err := addViaNATPMP(route.gateway, port, port, protocol); err == nil {
		mapping := &PortMapping{InstanceID: instanceID, ExternalPort: port, InternalPort: port, Protocol: protocol, Method: "natpmp"}
		if err := m.mappings.Create(ctx, mapping); err != nil {
			return nil, nil, err
		}
		m.startNATPMPRenewal(mapping.ID, port, protocol)
		return mapping, nil, nil
	} else {
		log.Printf("network: nat-pmp port mapping failed, falling back to manual instructions: %v", err)
	}

	return nil, &ManualInfo{LocalIP: route.localIP.String(), InternalPort: port, ExternalPort: port, Protocol: protocol}, nil
}

// Remove tears down mapping via whichever method created it (a no-op
// error from the router side is logged, not returned -- the operator asked
// to stop exposing this port, so the local record should go away
// regardless of whether the router still had the lease).
func (m *Manager) Remove(ctx context.Context, mapping *PortMapping) error {
	switch mapping.Method {
	case "upnp":
		if err := deleteViaUPnP(ctx, mapping.ExternalPort, mapping.Protocol); err != nil {
			log.Printf("network: upnp DeletePortMapping for %s: %v (removing local record anyway)", mapping.ID, err)
		}
	case "natpmp":
		m.stopNATPMPRenewal(mapping.ID)
		route, err := defaultRoute(ctx)
		if err != nil {
			log.Printf("network: determine gateway to remove nat-pmp mapping %s: %v (removing local record anyway)", mapping.ID, err)
		} else if err := deleteViaNATPMP(route.gateway, mapping.InternalPort, mapping.Protocol); err != nil {
			log.Printf("network: nat-pmp delete for %s: %v (removing local record anyway)", mapping.ID, err)
		}
	}
	return m.mappings.Delete(ctx, mapping.ID)
}

// startNATPMPRenewal re-applies a NAT-PMP mapping on a timer so it
// survives past its lifetime (see natPMPLifetimeSeconds) -- UPnP mappings
// don't need this (leaseDuration=0 there means "forever", not "now").
// Keyed by mappingID so several mappings (web UI port, proxy game port,
// independently-exposed servers' game ports) can each have their own
// renewal loop running concurrently; starting one for an ID that already
// has one cancels the old loop first.
func (m *Manager) startNATPMPRenewal(mappingID string, port int, protocol string) {
	m.stopNATPMPRenewal(mappingID)
	ctx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.renewals[mappingID] = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(natPMPRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				route, err := defaultRoute(ctx)
				if err != nil {
					log.Printf("network: nat-pmp renewal for %s: determine gateway: %v", mappingID, err)
					continue
				}
				if err := addViaNATPMP(route.gateway, port, port, protocol); err != nil {
					log.Printf("network: nat-pmp renewal for %s failed (will retry): %v", mappingID, err)
				}
			}
		}
	}()
}

func (m *Manager) stopNATPMPRenewal(mappingID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.renewals[mappingID]; ok {
		cancel()
		delete(m.renewals, mappingID)
	}
}
