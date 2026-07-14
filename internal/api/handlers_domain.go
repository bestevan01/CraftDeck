package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"craftdeck/internal/ddns"
	"craftdeck/internal/dns"
	"craftdeck/internal/secrets"
)

// handleGetDomainSettings returns the current domain registration (FR-26),
// or a zero-value response (registered: false) if none exists yet.
func (s *Server) handleGetDomainSettings(w http.ResponseWriter, r *http.Request) {
	config, err := s.domains.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if config == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"registered": false})
		return
	}
	writeJSON(w, http.StatusOK, config)
}

type setDomainSettingsRequest struct {
	Kind     string `json:"kind"` // "main_domain" or "free_subdomain"
	Provider string `json:"provider"`
	Hostname string `json:"hostname"`
	// Token is the provider API credential -- DuckDNS's token for
	// kind=free_subdomain (FR-26c), or a Cloudflare API token scoped to this
	// domain's zone for kind=main_domain (FR-28~31). Ignored for a
	// monitor-only free-subdomain provider (ipTime, FR-26e). Encrypted
	// before being persisted and never echoed back.
	Token string `json:"token,omitempty"`
}

// mainDomainProvider is the only owned-main-domain DNS provider FR-28~31's
// automation actually talks to right now (see internal/dns) -- mirrors
// internal/ddns's free-subdomain provider allowlist. "provider" stays a
// user-supplied field (rather than a hardcoded constant) so the UI/API
// shape doesn't need to change again once a second provider is added.
const mainDomainProvider = "cloudflare"

// handleSetDomainSettings registers (replacing any existing) the
// operator's domain connection method. For kind=free_subdomain, the
// provider must be one FR-26 actually supports (internal/ddns.
// SupportedFreeProviders) and an immediate reconcile pass runs right away
// (internal/ddns.Manager.Reconcile) so the operator gets instant feedback
// (e.g. DuckDNS rejecting a bad token) instead of waiting up to
// ReconcileInterval. For kind=main_domain, the token must be a Cloudflare
// API token that can actually see the requested zone (internal/dns.
// VerifyZoneAccess) -- FR-31's ownership check: a token scoped to one zone
// (as the UI instructs operators to create) can only pass this if they
// really do control that zone in their own Cloudflare account, which is
// what a manually-confirmed TXT record would otherwise have been trying to
// establish.
func (s *Server) handleSetDomainSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req setDomainSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	kind := ddns.Kind(req.Kind)
	if kind != ddns.KindMainDomain && kind != ddns.KindFreeSubdomain {
		http.Error(w, `kind must be "main_domain" or "free_subdomain"`, http.StatusBadRequest)
		return
	}
	hostname := strings.TrimSpace(req.Hostname)
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}
	provider := strings.TrimSpace(req.Provider)

	var mode ddns.Mode
	var tokenEncrypted, zoneID string
	if kind == ddns.KindFreeSubdomain {
		if !ddns.IsSupportedFreeProvider(provider) {
			http.Error(w, fmt.Sprintf(
				"unsupported free-subdomain provider %q (supported: %s)",
				provider, strings.Join(ddns.SupportedFreeProviders, ", ")), http.StatusBadRequest)
			return
		}
		if ddns.IsMonitorOnly(provider) {
			mode = ddns.ModeMonitor
			// No token: ipTime has no third-party renewal API at all
			// (FR-26b) -- this hostname is watch-only (FR-26e).
		} else {
			mode = ddns.ModeActive
			if strings.TrimSpace(req.Token) == "" {
				http.Error(w, fmt.Sprintf("%s requires an API token", provider), http.StatusBadRequest)
				return
			}
			encrypted, err := secrets.Encrypt(s.masterKey, req.Token)
			if err != nil {
				http.Error(w, "failed to encrypt provider token: "+err.Error(), http.StatusInternalServerError)
				return
			}
			tokenEncrypted = encrypted
		}
	} else {
		if !strings.EqualFold(provider, mainDomainProvider) {
			http.Error(w, fmt.Sprintf(
				"main_domain currently only supports %q as the provider", mainDomainProvider), http.StatusBadRequest)
			return
		}
		token := strings.TrimSpace(req.Token)
		if token == "" {
			http.Error(w, "a Cloudflare API token (scoped to this domain's zone) is required", http.StatusBadRequest)
			return
		}
		resolvedZoneID, err := dns.VerifyZoneAccess(ctx, token, hostname)
		if err != nil {
			http.Error(w, "couldn't verify domain ownership via Cloudflare: "+err.Error(), http.StatusBadRequest)
			return
		}
		zoneID = resolvedZoneID
		encrypted, err := secrets.Encrypt(s.masterKey, token)
		if err != nil {
			http.Error(w, "failed to encrypt provider token: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tokenEncrypted = encrypted
		mode = ddns.ModeActive
	}

	config, err := s.domains.Set(ctx, kind, provider, hostname, mode, tokenEncrypted, zoneID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if kind == ddns.KindFreeSubdomain {
		if err := s.ddnsManager.Reconcile(ctx); err != nil {
			// The registration itself succeeded and is saved -- surface the
			// reconcile failure as a warning the operator can act on (bad
			// token, hostname typo, provider unreachable) rather than
			// rolling back a config that might just need a retry once the
			// underlying issue is fixed.
			http.Error(w, "domain registered, but the first sync failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		if refreshed, err := s.domains.Get(ctx); err == nil && refreshed != nil {
			config = refreshed
		}
	} else {
		if err := s.ReconcileProxyMode(ctx); err != nil {
			http.Error(w, "domain registered, but failed to reconcile proxy mode: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// ReconcileProxyMode only creates/registers the proxy -- it doesn't
		// start it or open its port. Mirrors the same two calls main.go makes
		// at startup (see reconcileInstances there); without these, a freshly
		// registered main domain leaves the proxy instance sitting stopped
		// and unreachable until the next daemon restart.
		if err := s.EnsureProxyRunning(ctx); err != nil {
			http.Error(w, "domain registered, but failed to start the proxy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.ReconcileGamePorts(ctx); err != nil {
			http.Error(w, "domain registered, but failed to reconcile port forwarding: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, config)
}

// handleDeleteDomainSettings unregisters whatever domain is currently
// connected and reconciles Velocity's mode accordingly (see
// handleSetDomainSettings).
func (s *Server) handleDeleteDomainSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.domains.Clear(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.ReconcileProxyMode(ctx); err != nil {
		http.Error(w, "domain unregistered, but failed to reconcile proxy mode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
