package process

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// instanceUsername derives a stable, valid Linux system username from an
// instance ID. Usernames must start with a letter, so "mc-" is prefixed;
// only the first 12 hex chars of the UUID are used to stay comfortably
// under useradd's length limit.
func instanceUsername(instanceID string) string {
	id := strings.ReplaceAll(instanceID, "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return "mc-" + id
}

// EnsureInstanceUser creates a dedicated, login-disabled system user for an
// instance if one doesn't already exist.
//
// This replaces an earlier design that used systemd's DynamicUser=yes +
// StateDirectory=. VERIFIED against a real Raspberry Pi OS install: when
// files are provisioned into the work directory before the unit's first
// start (which is how loader downloads/EULA/server.properties
// provisioning works here), DynamicUser's "public/private StateDirectory"
// migration logged that it moved the directory but the process still
// failed at CHDIR with Permission denied. A fixed per-instance user that
// the root panel process chowns the directory to ahead of time (see
// ChownRecursive) avoids that whole migration path.
func EnsureInstanceUser(ctx context.Context, instanceID string) (string, error) {
	username := instanceUsername(instanceID)
	if err := exec.CommandContext(ctx, "id", username).Run(); err == nil {
		return username, nil // already exists
	}
	cmd := exec.CommandContext(ctx, "useradd",
		"--system", "--no-create-home", "--shell", "/usr/sbin/nologin", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("useradd %s: %w: %s", username, err, out)
	}
	return username, nil
}

// RemoveInstanceUser deletes the per-instance system user created by
// EnsureInstanceUser. Safe to call even if the user doesn't exist.
func RemoveInstanceUser(ctx context.Context, instanceID string) error {
	username := instanceUsername(instanceID)
	if err := exec.CommandContext(ctx, "id", username).Run(); err != nil {
		return nil // already gone
	}
	if out, err := exec.CommandContext(ctx, "userdel", username).CombinedOutput(); err != nil {
		return fmt.Errorf("userdel %s: %w: %s", username, err, out)
	}
	return nil
}

// ChownRecursive gives the named user (and its matching group, created
// alongside it by useradd --system) ownership of dir and everything in it.
// Called once at provisioning time, before the instance's first start;
// files the Minecraft process creates afterward inherit that ownership
// naturally since the process itself runs as that user.
func ChownRecursive(ctx context.Context, dir, username string) error {
	cmd := exec.CommandContext(ctx, "chown", "-R", username+":"+username, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chown -R %s %s: %w: %s", username, dir, err, out)
	}
	return nil
}
