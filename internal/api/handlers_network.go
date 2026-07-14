package api

import (
	"encoding/json"
	"net/http"

	"craftdeck/internal/network"
)

// networkSettingsResponse is shared by handleGetNetworkSettings and
// handleSetNetworkSettings. WebMapping/ManualInfo describe the web UI
// port's own mapping specifically; each instance's own game-port mapping
// is tracked separately (see ReconcileGamePorts, handleListPortMappings).
type networkSettingsResponse struct {
	WANEnabled bool                 `json:"wan_enabled"`
	WebMapping *network.PortMapping `json:"web_mapping,omitempty"`
	ManualInfo *network.ManualInfo  `json:"manual_info,omitempty"`
}

type networkAddressesResponse struct {
	LocalIP  string `json:"local_ip"`
	PublicIP string `json:"public_ip,omitempty"`
}

// handleGetNetworkAddresses backs the instance detail page's "접속 주소"
// copy buttons -- the LAN address always (for players on the same home
// network), and the public address only when FR-21's WAN toggle is on
// (showing it otherwise would be misleading: without that toggle nothing
// is actually forwarded, so the address wouldn't work).
func (s *Server) handleGetNetworkAddresses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	localIP, err := network.LocalIP(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := networkAddressesResponse{LocalIP: localIP}

	if settings, err := s.networkSettings.Get(ctx); err == nil && settings.WANEnabled {
		if publicIP, err := network.FetchPublicIP(ctx); err == nil {
			resp.PublicIP = publicIP
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetNetworkSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := s.networkSettings.Get(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := networkSettingsResponse{WANEnabled: settings.WANEnabled}
	if settings.WANEnabled {
		if mapping, err := s.portMappings.GetWebMapping(ctx); err == nil {
			resp.WebMapping = mapping
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type setNetworkSettingsRequest struct {
	WANEnabled bool `json:"wan_enabled"`
}

// handleSetNetworkSettings implements FR-21/22/23/25: one "외부 접속 허용"
// toggle covers both the web UI port and every directly-reachable
// Minecraft game port. Turning it on maps the web UI port (UPnP then
// NAT-PMP, network.Manager.Ensure) and reconciles game-port mappings for
// every currently-running proxy/independently-exposed instance
// (ReconcileGamePorts); turning it off tears both down. Re-sending "on"
// while already on is treated as a deliberate retry (e.g. after the
// operator fixed their router) rather than a no-op, so it always re-runs
// Ensure for the web UI port.
func (s *Server) handleSetNetworkSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req setNetworkSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// FR-38: turning WAN exposure on is held back until the account has 2FA
	// enrolled -- checked here (not just documented as a should) because
	// once this succeeds, FR-37 makes the admin login itself require a TOTP
	// code, and an operator without a working authenticator would lock
	// themselves out of their own panel. Only gates the "turn on" path;
	// turning WAN off is always allowed no matter what.
	if req.WANEnabled {
		user, ok := s.currentUser(r)
		if !ok || !user.TOTPEnabled {
			http.Error(w, "two-factor authentication must be set up before enabling WAN exposure -- see 계정 설정 > 2단계 인증", http.StatusPreconditionFailed)
			return
		}
	}

	if !req.WANEnabled {
		if mapping, err := s.portMappings.GetWebMapping(ctx); err == nil {
			if err := s.netManager.Remove(ctx, mapping); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err := s.networkSettings.SetWANEnabled(ctx, false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.ReconcileGamePorts(ctx); err != nil {
			http.Error(w, "web UI exposure disabled, but failed to reconcile game ports: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, networkSettingsResponse{})
		return
	}

	// Clear any existing web-UI mapping first so re-enabling (including a
	// retry) doesn't accumulate duplicate port_mappings rows.
	if mapping, err := s.portMappings.GetWebMapping(ctx); err == nil {
		_ = s.netManager.Remove(ctx, mapping)
	}
	mapping, manual, err := s.netManager.Ensure(ctx, nil, s.webUIPort, "tcp", "CraftDeck Web UI")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.networkSettings.SetWANEnabled(ctx, true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.ReconcileGamePorts(ctx); err != nil {
		http.Error(w, "web UI port mapped, but failed to reconcile game ports: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, networkSettingsResponse{
		WANEnabled: true,
		WebMapping: mapping,
		ManualInfo: manual,
	})
}

// handleListPortMappings backs FR-24's review list -- only what CraftDeck
// itself registered, never a full dump of the router's own port-forwarding
// table (which could include mappings for unrelated devices).
func (s *Server) handleListPortMappings(w http.ResponseWriter, r *http.Request) {
	list, err := s.portMappings.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleDeletePortMapping backs FR-24's per-rule revoke action. Deleting
// the web UI's own mapping this way (rather than through the settings
// toggle) still flips wan_enabled back off, so the toggle and the mapping
// list can't drift apart. Deleting an instance's game-port mapping this
// way, on the other hand, doesn't touch wan_enabled -- ReconcileGamePorts
// would just re-create it the next time that instance (re)starts, since
// the toggle itself is still on; this is only meant as a one-off "close
// this port right now" action (e.g. before taking a server down for
// maintenance without fully stopping it).
func (s *Server) handleDeletePortMapping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mapping, err := s.portMappings.Get(ctx, r.PathValue("id"))
	if err != nil {
		http.Error(w, "port mapping not found", http.StatusNotFound)
		return
	}
	if err := s.netManager.Remove(ctx, mapping); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if mapping.InstanceID == nil {
		if err := s.networkSettings.SetWANEnabled(ctx, false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
