// Package swap manages an optional disk-backed swap file, entirely
// independent of whatever RAM-based swap (e.g. Raspberry Pi OS's
// systemd-zram-generator/"rpi-swap") the base OS may already have active --
// the two coexist (the kernel already prefers real RAM first, then
// whichever swap has the higher priority, which is zram by default) rather
// than conflicting. This exists for a Pi with a fast, spacious NVMe/SSD
// where trading a bit of disk space for a safety margin beyond RAM+zram is
// worth it; it does nothing to help a Pi still booting off a slow SD card,
// which is why it's an opt-in the operator sizes themselves rather than
// something CraftDeck enables automatically.
package swap

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// filename is deliberately inside CraftDeck's own data directory (not
// /swapfile at the root) so it's unambiguous which swap file is
// CraftDeck's, and so it travels with the rest of CraftDeck's data rather
// than being an orphaned file elsewhere the operator has to remember about
// separately.
const filename = "swapfile"

// minFreeDiskMarginMB is kept free on top of the requested swap size itself
// -- creating/growing a swap file to the very last byte of free space would
// leave no room for anything else (logs, world saves, backups) to grow
// afterward.
const minFreeDiskMarginMB = 1024

func swapFilePath(dataDir string) string {
	return filepath.Join(dataDir, filename)
}

// Info reports the current state of CraftDeck's managed swap file.
type Info struct {
	// Supported is false when dataDir lives on storage a disk-backed swap
	// file would actively hurt (an SD card) rather than just not need --
	// see IsSlowStorage. The frontend hides the whole feature in that case
	// rather than just disabling controls, since there's nothing here an
	// operator on that hardware should be encouraged to turn on at all.
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
	SizeMB    int  `json:"size_mb"`
	UsedMB    int  `json:"used_mb"`
	// FreeDiskMB is how much space is left on the filesystem backing the
	// swap file's directory, on top of the swap file's own size -- what an
	// operator actually has headroom to grow it into.
	FreeDiskMB int `json:"free_disk_mb"`
}

// Status reports whether CraftDeck's swap file exists/is active, and how
// much headroom is left to grow it.
func Status(ctx context.Context, dataDir string) (*Info, error) {
	path := swapFilePath(dataDir)
	info := &Info{}

	slow, err := IsSlowStorage(ctx, dataDir)
	if err != nil {
		return nil, err
	}
	info.Supported = !slow

	if fi, err := os.Stat(path); err == nil {
		info.SizeMB = int(fi.Size() / (1024 * 1024))
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat swap file: %w", err)
	}

	usedKB, active, err := activeSwapUsageKB(path)
	if err != nil {
		return nil, err
	}
	info.Enabled = active
	info.UsedMB = usedKB / 1024

	freeMB, err := freeDiskMB(dataDir)
	if err != nil {
		return nil, err
	}
	// The swap file's own bytes count as "used" from the filesystem's
	// point of view, but they're exactly what would be freed up if the
	// operator shrunk/removed the swap file -- so from the operator's
	// perspective of "how much bigger could I make this", that space is
	// available too.
	info.FreeDiskMB = freeMB + info.SizeMB
	return info, nil
}

// activeSwapUsageKB scans /proc/swaps for path, returning its current used
// size (in KB) and whether it's active at all.
func activeSwapUsageKB(path string) (usedKB int, active bool, err error) {
	f, err := os.Open("/proc/swaps")
	if err != nil {
		return 0, false, fmt.Errorf("read /proc/swaps: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		// Columns: Filename, Type, Size, Used, Priority -- Used (actually
		// swapped-out pages currently resident in this swap file/device) is
		// field index 3, not 2 (Size, the swap file's total capacity, which
		// this used to read by mistake -- confirmed on real hardware: the
		// UI showed the swap bar as fully used the moment the file was
		// created, regardless of actual memory pressure).
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[0] != path {
			continue
		}
		used, err := strconv.Atoi(fields[3])
		if err != nil {
			return 0, false, fmt.Errorf("parse /proc/swaps: %w", err)
		}
		return used, true, nil
	}
	return 0, false, scanner.Err()
}

// freeDiskMB shells out to `df` rather than the raw statfs(2) syscall for
// the same portability reason as internal/api's diskUsageMB (its
// Statfs_t layout differs across platforms this project is developed on
// but never actually runs craftdeckd on).
func freeDiskMB(path string) (int, error) {
	out, err := exec.Command("df", "-Pk", path).Output()
	if err != nil {
		return 0, fmt.Errorf("df: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output: %q", out)
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df output: %q", out)
	}
	availKB, err := strconv.Atoi(fields[3])
	if err != nil {
		return 0, err
	}
	return availKB / 1024, nil
}

// IsSlowStorage reports whether dataDir lives on storage a disk-backed
// swap file would actively hurt rather than just not particularly help --
// concretely, a Raspberry Pi's SD card or eMMC (Linux's mmcblk* block
// devices), where swap's random-write pattern is both slow and wears the
// media down faster than a normal write workload would. NVMe (nvme*) and
// SATA/USB-attached (sd*) storage are both treated as fast enough --
// findmnt resolves whatever's actually mounted under dataDir (a PCIe HAT
// NVMe SSD, in the case this was written for) rather than assuming
// anything about the specific hardware.
func IsSlowStorage(ctx context.Context, dataDir string) (bool, error) {
	out, err := exec.CommandContext(ctx, "findmnt", "-no", "SOURCE", "--target", dataDir).Output()
	if err != nil {
		return false, fmt.Errorf("findmnt: %w", err)
	}
	return strings.Contains(string(out), "mmcblk"), nil
}

// Set creates CraftDeck's swap file if it doesn't exist, or replaces it
// with a new one of sizeMB if it does (there's no in-place "resize" for a
// swap file -- it has to be turned off, recreated, and turned back on).
// sizeMB <= 0 is rejected; call Disable to remove it instead.
func Set(ctx context.Context, dataDir string, sizeMB int) error {
	if sizeMB <= 0 {
		return fmt.Errorf("size must be positive (use Disable to remove the swap file)")
	}

	path := swapFilePath(dataDir)
	current, err := Status(ctx, dataDir)
	if err != nil {
		return err
	}
	if !current.Supported {
		return fmt.Errorf("swap file is disabled on this storage (looks like an SD card -- see IsSlowStorage)")
	}
	// current.FreeDiskMB already adds back the existing swap file's own
	// size (see Status), so this correctly reflects "how big could the
	// file become" whether we're creating one fresh or growing/shrinking
	// an existing one.
	if sizeMB+minFreeDiskMarginMB > current.FreeDiskMB {
		return fmt.Errorf("not enough free disk space: %dMB requested (+%dMB safety margin) but only %dMB available",
			sizeMB, minFreeDiskMarginMB, current.FreeDiskMB)
	}

	if current.Enabled {
		if err := runCommand(ctx, "swapoff", path); err != nil {
			return fmt.Errorf("swapoff existing swap file: %w", err)
		}
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing swap file: %w", err)
	}

	if err := runCommand(ctx, "fallocate", "-l", fmt.Sprintf("%dM", sizeMB), path); err != nil {
		// Some filesystems don't support fallocate for swap files (it needs
		// to produce a file with no holes) -- dd is slower but always works.
		if ddErr := runCommand(ctx, "dd", "if=/dev/zero", "of="+path, "bs=1M", fmt.Sprintf("count=%d", sizeMB)); ddErr != nil {
			return fmt.Errorf("allocate swap file (fallocate: %v, dd fallback: %w)", err, ddErr)
		}
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod swap file: %w", err)
	}
	if err := runCommand(ctx, "mkswap", path); err != nil {
		return fmt.Errorf("mkswap: %w", err)
	}
	if err := runCommand(ctx, "swapon", path); err != nil {
		return fmt.Errorf("swapon: %w", err)
	}
	if err := ensureFstabEntry(path); err != nil {
		return fmt.Errorf("swap file is active, but persisting it across reboots failed: %w", err)
	}
	return nil
}

// Disable turns off and removes CraftDeck's swap file, and drops its
// /etc/fstab entry so a reboot doesn't try to re-activate a file that no
// longer exists.
func Disable(ctx context.Context, dataDir string) error {
	path := swapFilePath(dataDir)
	if _, active, err := activeSwapUsageKB(path); err != nil {
		return err
	} else if active {
		if err := runCommand(ctx, "swapoff", path); err != nil {
			return fmt.Errorf("swapoff: %w", err)
		}
	}
	if err := removeFstabEntry(path); err != nil {
		return fmt.Errorf("remove fstab entry: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove swap file: %w", err)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

const fstabPath = "/etc/fstab"

// ensureFstabEntry appends path's swap entry to /etc/fstab if it isn't
// already there, so the swap file survives a reboot instead of silently
// vanishing until the operator notices and re-applies the setting.
func ensureFstabEntry(path string) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == path {
			return nil // already present
		}
	}
	f, err := os.OpenFile(fstabPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s none swap sw 0 0\n", path)
	return err
}

// removeFstabEntry drops path's line from /etc/fstab, if present.
func removeFstabEntry(path string) error {
	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == path {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	if !changed {
		return nil
	}
	return os.WriteFile(fstabPath, []byte(strings.Join(kept, "\n")), 0o644)
}
