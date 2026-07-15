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
	// AllowSwap lets this instance's cgroup page out to swap instead of
	// being OOM-killed once it exceeds physical RAM -- the caller should
	// only set this when CraftDeck's own swap file (internal/swap) is
	// actually enabled, so a memory allocation above physical RAM has
	// somewhere real to go. False (the long-standing default) keeps a
	// runaway instance from dragging the whole system into slow,
	// heavily-swapped territory; see Start's MemorySwapMax handling.
	AllowSwap bool
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
	// A unit that most recently exited via a signal (e.g. a graceful RCON
	// "stop" racing with our own fallback systemctl stop) is left loaded in
	// "failed" state indefinitely -- systemd-run then refuses to create a
	// new transient unit with the same name ("Unit ... was already loaded"),
	// which is exactly what blocked restarting an instance after a stop.
	// Clear any such leftover state before asking for a fresh unit.
	_ = exec.CommandContext(ctx, "systemctl", "reset-failed", unitName(spec.InstanceID)).Run()

	// MemorySwapMax=0 (the default) keeps this instance's cgroup from ever
	// paging out to swap at all -- once physical RAM allocated to it runs
	// out, the kernel OOM-kills it outright rather than letting it slow to
	// a crawl swapping a JVM heap. When the operator has actually turned on
	// CraftDeck's own swap file and wants to let an instance's allocation
	// exceed physical RAM, AllowSwap lifts this cap entirely (rather than
	// trying to carve up a fair per-instance share of one shared swap
	// file/device, which the kernel's own swap accounting already does
	// naturally across whichever cgroups are actually using it).
	memorySwapMax := "0"
	if spec.AllowSwap {
		memorySwapMax = "infinity"
	}
	args := []string{
		"--unit=" + unitName(spec.InstanceID),
		"--property=User=" + spec.Username,
		"--property=Group=" + spec.Username,
		"--property=WorkingDirectory=" + spec.WorkDir,
		"--property=MemorySwapMax=" + memorySwapMax,
		"--property=Restart=no",
		// requirements.md FR-45: systemd already sends SIGTERM (triggering
		// Minecraft's own save-all-then-exit shutdown hook) to every running
		// unit during a normal `systemctl stop` *and* during a full system
		// reboot/shutdown -- no separate hook is needed for the "operator
		// manually reboots" case the apt hook (see packaging/scripts/
		// apt-hook) deliberately left out of scope, since that path was
		// never actually about apt at all. The real gap was the default
		// stop timeout (90s) being too short for a slow SD-card save on a
		// Pi to finish before systemd gives up and sends SIGKILL instead,
		// risking a corrupted world file mid-write. 5 minutes is generous
		// enough for that while still eventually forcing a stop if the
		// process is genuinely hung, so a reboot doesn't wait forever.
		"--property=TimeoutStopSec=300",
		// Automatically unload the unit as soon as it stops, whether it
		// exits cleanly or via signal/failure, so it never lingers in
		// "loaded (failed)" state and blocks a future restart under the
		// same instance ID.
		"--property=CollectMode=inactive-or-failed",
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
