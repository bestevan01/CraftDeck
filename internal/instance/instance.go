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
	ID               string
	Name             string
	Kind             Kind
	Loader           string // vanilla|paper|purpur|forge|fabric|velocity|bungeecord
	LoaderVersion    string
	MCVersion        string
	JavaMajor        int // 8, 17, or 21; zero for proxy instances
	GamePort         int
	RCONPort         int
	RCONPassword     string // stored encrypted at rest; decrypted in memory only
	CPUQuotaPercent  int
	MemoryMaxMB      int
	WorkDir          string
	Status           Status
	CreatedAt        time.Time
}
