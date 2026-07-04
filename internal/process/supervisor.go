// Package process starts and supervises Minecraft server/proxy instances as
// transient systemd units. Per ARCHITECTURE.md section 5.1 (as revised
// after real-hardware testing -- see instanceuser.go), each instance runs
// under its own fixed, login-disabled system user rather than a shared
// account, so a compromised plugin/mod in one instance cannot read another
// instance's world data.
package process

import (
	"context"
	"fmt"
	"os/exec"
)

// StartSpec carries everything Supervisor.Start needs to build the
// systemd-run invocation for one instance.
type StartSpec struct {
	InstanceID string
	WorkDir    string
	// Username is the per-instance system user (see EnsureInstanceUser)
	// that owns WorkDir; the process runs as this user, not root.
	Username        string
	JavaBinary      string // e.g. /usr/lib/jvm/temurin-17-jre-arm64/bin/java
	JavaArgs        []string
	CPUQuotaPercent int // 0 means "unset, no limit"
	MemoryMaxMB     int // 0 means "unset, no limit"
}

func unitName(instanceID string) string {
	return "craftdeck-instance-" + instanceID
}

type Supervisor struct{}

func NewSupervisor() *Supervisor {
	return &Supervisor{}
}

// Start launches the instance as a transient systemd unit. It returns once
// systemd has accepted the unit; it does not wait for the Minecraft process
// to finish booting (callers should watch IsActive or the RCON connection
// for that).
func (s *Supervisor) Start(ctx context.Context, spec StartSpec) error {
	args := []string{
		"--unit=" + unitName(spec.InstanceID),
		"--property=User=" + spec.Username,
		"--property=Group=" + spec.Username,
		"--property=WorkingDirectory=" + spec.WorkDir,
		"--property=MemorySwapMax=0",
		"--property=Restart=no",
	}
	if spec.MemoryMaxMB > 0 {
		args = append(args, fmt.Sprintf("--property=MemoryMax=%dM", spec.MemoryMaxMB))
	}
	if spec.CPUQuotaPercent > 0 {
		args = append(args, fmt.Sprintf("--property=CPUQuota=%d%%", spec.CPUQuotaPercent))
	}
	args = append(args, "--", spec.JavaBinary)
	args = append(args, spec.JavaArgs...)

	cmd := exec.CommandContext(ctx, "systemd-run", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run start %s: %w: %s", spec.InstanceID, err, out)
	}
	return nil
}

// Stop asks systemd to terminate the unit. Callers should prefer sending an
// RCON "stop" command first for a graceful shutdown; Stop is the fallback
// for instances that don't respond.
func (s *Supervisor) Stop(ctx context.Context, instanceID string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "stop", unitName(instanceID))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop %s: %w: %s", instanceID, err, out)
	}
	return nil
}

// IsActive reports whether the unit is currently running, for reconciling
// instances.status against systemd's own view after a supervisor restart.
func (s *Supervisor) IsActive(ctx context.Context, instanceID string) (bool, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", unitName(instanceID))
	out, err := cmd.Output()
	if err != nil {
		// systemctl exits non-zero for inactive/failed units; that's a
		// valid (negative) answer, not an execution failure.
		return false, nil
	}
	return string(out) == "active\n", nil
}
