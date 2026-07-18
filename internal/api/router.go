// Package api wires the REST endpoints and WebSocket console handler
// described in ARCHITECTURE.md section 4. It uses the standard library's
// http.ServeMux (Go 1.22+ method+path patterns) rather than an external
// router, matching the project's "minimize apt/runtime dependencies"
// philosophy (NFR-9) at the code level too.
package api

import (
	"net"
	"net/http"

	"craftdeck/internal/auth"
	"craftdeck/internal/backup"
	"craftdeck/internal/ddns"
	"craftdeck/internal/instance"
	"craftdeck/internal/network"
	"craftdeck/internal/plugin"
	"craftdeck/internal/process"
	"craftdeck/internal/rcon"
)

type Server struct {
	instances  *instance.Repository
	supervisor *process.Supervisor
	rconMgr    *rcon.Manager
	users      *auth.Repository
	backups    *backup.Repository
	plugins    *plugin.Repository
	// dataDir roots per-instance work directories (dataDir/instances/<id>);
	// see internal/config for how it's configured (CRAFTDECK_DATA_DIR).
	dataDir string

	// networkSettings/portMappings/netManager back FR-21~25's "외부 접속
	// 허용" toggle and UPnP/NAT-PMP port-forwarding automation (see
	// internal/network and handlers_network.go). webUIPort is the port
	// craftdeckd itself listens on (parsed from config.ListenAddr), i.e.
	// what gets mapped when the web-UI-exposure toggle is turned on.
	networkSettings *network.SettingsRepository
	portMappings    *network.MappingRepository
	netManager      *network.Manager
	webUIPort       int

	// domains/ddnsManager back FR-26~31 -- domains is whether an owned
	// domain is registered (FR-1f gates Velocity on it, ReconcileProxyMode
	// in handlers_proxy.go), and ddnsManager is the free-subdomain
	// active-renewal/monitor reconciler (FR-26/30, internal/ddns).
	domains     *ddns.Repository
	ddnsManager *ddns.Manager
	// masterKey encrypts DDNS provider tokens at rest (FR-31,
	// internal/secrets) -- loaded once at startup from config.MasterKeyPath.
	masterKey []byte
}

func NewServer(
	instances *instance.Repository,
	supervisor *process.Supervisor,
	rconMgr *rcon.Manager,
	users *auth.Repository,
	backups *backup.Repository,
	plugins *plugin.Repository,
	dataDir string,
	networkSettings *network.SettingsRepository,
	portMappings *network.MappingRepository,
	netManager *network.Manager,
	webUIPort int,
	domains *ddns.Repository,
	ddnsManager *ddns.Manager,
	masterKey []byte,
) *Server {
	return &Server{
		instances:       instances,
		supervisor:      supervisor,
		rconMgr:         rconMgr,
		users:           users,
		backups:         backups,
		plugins:         plugins,
		dataDir:         dataDir,
		networkSettings: networkSettings,
		portMappings:    portMappings,
		domains:         domains,
		ddnsManager:     ddnsManager,
		netManager:      netManager,
		webUIPort:       webUIPort,
		masterKey:       masterKey,
	}
}

// publicPaths lists the exact /api/... paths reachable without a valid
// session -- everything else under /api/ requires one (see requireAuth).
// The embedded SPA shell itself (index.html, JS/CSS bundle) is served from
// a completely separate handler in cmd/craftdeckd/main.go and was never
// wrapped by this, so the login/setup page can always load.
var publicPaths = map[string]bool{
	"/api/system/health": true,
	"/api/auth/status":   true,
	"/api/auth/setup":    true,
	"/api/auth/login":    true,
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/system/health", s.handleHealth)
	mux.HandleFunc("GET /api/system/resources", s.handleSystemResources)
	mux.HandleFunc("GET /api/system/version", s.handleCraftdeckVersion)
	mux.HandleFunc("GET /api/system/swap", s.handleGetSwap)
	mux.HandleFunc("PUT /api/system/swap", s.handleSetSwap)
	mux.HandleFunc("DELETE /api/system/swap", s.handleDeleteSwap)

	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/auth/password", s.handleChangePassword)
	mux.HandleFunc("POST /api/auth/2fa/setup", s.handleTOTPSetup)
	mux.HandleFunc("POST /api/auth/2fa/verify", s.handleTOTPVerify)
	mux.HandleFunc("POST /api/auth/2fa/disable", s.handleTOTPDisable)
	mux.HandleFunc("POST /api/auth/2fa/backup-codes/regenerate", s.handleTOTPRegenerateBackupCodes)

	mux.HandleFunc("GET /api/loaders/vanilla/versions", s.handleListVanillaVersions)
	mux.HandleFunc("GET /api/loaders/paper/versions", s.handleListPaperVersions)
	mux.HandleFunc("GET /api/loaders/purpur/versions", s.handleListPurpurVersions)
	mux.HandleFunc("GET /api/loaders/folia/versions", s.handleListFoliaVersions)
	mux.HandleFunc("GET /api/loaders/pufferfish/versions", s.handleListPufferfishVersions)
	mux.HandleFunc("GET /api/loaders/leaf/versions", s.handleListLeafVersions)
	mux.HandleFunc("GET /api/loaders/fabric/versions", s.handleListFabricVersions)
	mux.HandleFunc("GET /api/loaders/neoforge/versions", s.handleListNeoForgeVersions)
	mux.HandleFunc("GET /api/loaders/{loader}/builds", s.handleListLoaderBuilds)

	mux.HandleFunc("GET /api/instances", s.handleListInstances)
	mux.HandleFunc("POST /api/instances", s.handleCreateInstance)
	mux.HandleFunc("GET /api/instances/{id}", s.handleGetInstance)
	mux.HandleFunc("PATCH /api/instances/{id}", s.handleUpdateInstance)
	mux.HandleFunc("DELETE /api/instances/{id}", s.handleDeleteInstance)
	mux.HandleFunc("POST /api/instances/{id}/jar", s.handleUploadServerJar)
	mux.HandleFunc("POST /api/instances/{id}/reinstall", s.handleReinstallLoader)
	mux.HandleFunc("POST /api/instances/{id}/start", s.handleStartInstance)
	mux.HandleFunc("POST /api/instances/{id}/stop", s.handleStopInstance)
	mux.HandleFunc("POST /api/instances/{id}/restart", s.handleRestartInstance)
	mux.HandleFunc("POST /api/instances/{id}/command", s.handleSendCommand)
	mux.HandleFunc("GET /api/instances/{id}/players", s.handleOnlinePlayers)
	mux.HandleFunc("GET /api/instances/{id}/bans", s.handleListBans)
	mux.HandleFunc("GET /api/instances/{id}/ops", s.handleListOps)
	mux.HandleFunc("GET /api/instances/{id}/whitelist", s.handleListWhitelist)
	mux.HandleFunc("GET /api/instances/{id}/settings", s.handleGetServerSettings)
	mux.HandleFunc("PUT /api/instances/{id}/settings", s.handleSetServerSettings)

	mux.HandleFunc("GET /api/instances/{id}/backups", s.handleListBackups)
	mux.HandleFunc("POST /api/instances/{id}/backups", s.handleCreateBackup)
	mux.HandleFunc("DELETE /api/instances/{id}/backups/{backupId}", s.handleDeleteBackup)
	mux.HandleFunc("POST /api/instances/{id}/backups/{backupId}/restore", s.handleRestoreBackup)

	mux.HandleFunc("GET /api/instances/{id}/world/info", s.handleWorldInfo)
	mux.HandleFunc("GET /api/instances/{id}/world/export", s.handleExportWorld)
	mux.HandleFunc("POST /api/instances/{id}/world/import", s.handleImportWorld)

	mux.HandleFunc("GET /api/instances/{id}/plugins/search", s.handleSearchPlugins)
	mux.HandleFunc("GET /api/instances/{id}/plugins", s.handleListPlugins)
	mux.HandleFunc("POST /api/instances/{id}/plugins", s.handleInstallPlugin)
	mux.HandleFunc("POST /api/instances/{id}/plugins/upload", s.handleUploadPlugin)
	mux.HandleFunc("PATCH /api/instances/{id}/plugins/{pluginId}", s.handleSetPluginEnabled)
	mux.HandleFunc("DELETE /api/instances/{id}/plugins/{pluginId}", s.handleDeletePlugin)

	mux.HandleFunc("GET /api/instances/{id}/proxy/backends", s.handleListProxyBackends)
	mux.HandleFunc("PUT /api/instances/{id}/proxy/backends", s.handleSetProxyBackends)
	mux.HandleFunc("GET /api/instances/{id}/proxy/secret", s.handleGetForwardingSecret)
	mux.HandleFunc("GET /api/instances/{id}/subdomain", s.handleGetServerSubdomain)
	mux.HandleFunc("PUT /api/instances/{id}/subdomain", s.handleSetServerSubdomain)
	mux.HandleFunc("POST /api/instances/{id}/proxy/register", s.handleRegisterBehindProxy)
	mux.HandleFunc("POST /api/instances/{id}/proxy/unregister", s.handleUnregisterFromProxy)
	mux.HandleFunc("GET /api/instances/{id}/files", s.handleListFiles)
	mux.HandleFunc("GET /api/instances/{id}/files/content", s.handleGetFileContent)
	mux.HandleFunc("PUT /api/instances/{id}/files/content", s.handleSetFileContent)
	mux.HandleFunc("GET /api/instances/{id}/files/download", s.handleDownloadFile)
	mux.HandleFunc("POST /api/instances/{id}/files/upload", s.handleUploadFile)
	mux.HandleFunc("PUT /api/instances/{id}/files/rename", s.handleRenameFile)
	mux.HandleFunc("DELETE /api/instances/{id}/files", s.handleDeleteFile)
	mux.HandleFunc("GET /api/proxy/status", s.handleGetProxyStatus)
	mux.HandleFunc("POST /api/proxy/upgrade", s.handleUpgradeProxy)

	mux.HandleFunc("GET /api/network/settings", s.handleGetNetworkSettings)
	mux.HandleFunc("GET /api/network/addresses", s.handleGetNetworkAddresses)
	mux.HandleFunc("PUT /api/network/settings", s.handleSetNetworkSettings)
	mux.HandleFunc("GET /api/network/port-mappings", s.handleListPortMappings)
	mux.HandleFunc("DELETE /api/network/port-mappings/{id}", s.handleDeletePortMapping)

	mux.HandleFunc("GET /api/domain/settings", s.handleGetDomainSettings)
	mux.HandleFunc("PUT /api/domain/settings", s.handleSetDomainSettings)
	mux.HandleFunc("DELETE /api/domain/settings", s.handleDeleteDomainSettings)

	mux.HandleFunc("GET /api/loaders/velocity/versions", s.handleListVelocityVersions)

	mux.HandleFunc("GET /api/instances/{id}/console", s.handleConsoleWebSocket)

	return s.requireAuth(mux)
}

// requireAuth gates every /api/ route except publicPaths behind a valid
// session cookie (requirements.md FR-32) -- but grants a LAN convenience
// bypass, and only while FR-21's "외부 접속 허용" toggle for the web UI
// port is off. craftdeckd listens directly on the web port with no reverse
// proxy in front, and the router's port forwarding preserves the original
// client source IP (NAT doesn't rewrite it), so r.RemoteAddr reliably
// reflects whether a request actually came in from the WAN or from a
// device on the home network.
//
// Once the operator turns wan_web_enabled on, the LAN bypass is withdrawn
// entirely (even same-network requests need to log in) -- this used to be
// a TODO ("auth should be required whenever that toggle is on, regardless
// of source IP, rather than inferred from the request's own source
// address"); network.SettingsRepository (see internal/network,
// handlers_network.go) is that toggle now.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPaths[r.URL.Path] || s.authBypassed(r) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.currentUser(r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authBypassed reports whether requireAuth should let r through without a
// session -- true only for a LAN-sourced request while FR-21's "외부 접속
// 허용" toggle is off. Shared with handleAuthStatus's "lan_bypass" field so
// the frontend's login-redirect decision can never drift from what
// requireAuth itself actually enforces.
func (s *Server) authBypassed(r *http.Request) bool {
	if !isLANRequest(r) {
		return false
	}
	// Fail safe: if the settings row can't be read for some reason, don't
	// silently grant the bypass on unverified state.
	settings, err := s.networkSettings.Get(r.Context())
	return err == nil && !settings.WANEnabled
}

// isLANRequest reports whether r's source address is a loopback or private
// (RFC 1918 / RFC 4193) address.
func isLANRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr // no port suffix present; use as-is
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
