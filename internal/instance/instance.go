// Package instance manages Minecraft server and proxy instances: the
// "instances" table plus the lifecycle operations (create/start/stop) that
// hand off to internal/process for the actual systemd-run supervision.
package instance

import "time"

type Kind string

const (
	KindServer Kind = "server"
	KindProxy  Kind = "proxy"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusCrashed  Status = "crashed"
)

// Instance mirrors the `instances` table (see internal/db/migrations/0001_init.sql).
type Instance struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Kind            Kind      `json:"kind"`
	Loader          string    `json:"loader"` // vanilla|paper|purpur|forge|fabric|velocity|bungeecord
	LoaderVersion   string    `json:"loader_version"`
	MCVersion       string    `json:"mc_version"`
	JavaMajor       int       `json:"java_major"` // 8, 17, or 21; zero for proxy instances
	GamePort        int       `json:"game_port"`
	RCONPort        int       `json:"rcon_port"`
	RCONPassword    string    `json:"-"` // never serialized: the browser has no legitimate use for it, only the backend's RCON dialer does
	CPUQuotaPercent int       `json:"cpu_quota_percent"`
	MemoryMaxMB     int       `json:"memory_max_mb"`
	WorkDir         string    `json:"work_dir"`
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	// ProxyOptOut is true once an operator has explicitly converted this
	// server to independent exposure (see handlers_proxy.go's
	// unregisterServerFromProxyCore) -- distinct from "never registered
	// yet". ReconcileProxyMode checks this before auto-registering a
	// not-currently-behind-the-proxy server, so it doesn't silently undo
	// that choice on every daemon restart.
	ProxyOptOut bool `json:"proxy_opt_out"`

	// LogStorage* control internal/gamelog's per-instance on-disk console
	// capture (independent of journald's own system-wide retention) --
	// whether it's on at all, and how accumulated log files get cleaned up.
	// LogRetentionMode is "unlimited" (never delete), "age" (delete files
	// older than LogRetentionDays), or "size" (delete oldest files once
	// total size exceeds LogRetentionMaxMB).
	LogStorageEnabled bool   `json:"log_storage_enabled"`
	LogRetentionMode  string `json:"log_retention_mode"`
	LogRetentionDays  int    `json:"log_retention_days"`
	LogRetentionMaxMB int    `json:"log_retention_max_mb"`
}
