package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"craftdeck/internal/swap"
	"craftdeck/internal/version"
)

// craftdeckAptPackagesURL is our own apt repository's package index for the
// architecture this daemon actually runs on (Raspberry Pi 4/5 are both
// arm64) -- checking it directly (rather than e.g. GitHub's releases API)
// means "update available" here always matches exactly what
// `apt update && apt upgrade craftdeck` would do, since it's the same file
// apt itself resolves against.
const craftdeckAptPackagesURL = "https://apt.apple-farm.online/dists/trixie/main/binary-arm64/Packages"

type craftdeckVersionResponse struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
}

// handleCraftdeckVersion reports craftdeckd's own version against the
// newest one published to the apt repository, so the UI can surface an
// "update available" notice the same way it already does for the Velocity
// proxy (handleGetProxyStatus). Unlike that one, a fetch failure here isn't
// fatal to the response -- it's a nice-to-have notice, not something
// callers are blocked on, so update_available just stays false.
func (s *Server) handleCraftdeckVersion(w http.ResponseWriter, r *http.Request) {
	resp := craftdeckVersionResponse{CurrentVersion: version.Version}
	if latest, err := fetchLatestCraftdeckVersion(r.Context()); err == nil {
		resp.LatestVersion = latest
		resp.UpdateAvailable = latest != "" && latest != version.Version
	}
	writeJSON(w, http.StatusOK, resp)
}

// fetchLatestCraftdeckVersion parses the "Version:" field out of the
// craftdeck stanza in our apt repository's Packages index.
func fetchLatestCraftdeckVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, craftdeckAptPackagesURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d fetching apt Packages index", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	inCraftdeckStanza := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "Package: craftdeck":
			inCraftdeckStanza = true
		case line == "":
			inCraftdeckStanza = false
		case inCraftdeckStanza && strings.HasPrefix(line, "Version: "):
			return strings.TrimPrefix(line, "Version: "), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("craftdeck package not found in apt Packages index")
}

// systemResources is the payload for GET /api/system/resources: covers both
// the memory cap the instance-settings slider needs and the live usage
// numbers shown on the resource-monitor panel of the instance list page.
type systemResources struct {
	CPUPercent    float64  `json:"cpu_percent"`
	CPUCount      int      `json:"cpu_count"`
	CPUTempC      *float64 `json:"cpu_temp_c,omitempty"`
	TotalMemoryMB int      `json:"total_memory_mb"`
	UsedMemoryMB  int      `json:"used_memory_mb"`
	TotalDiskMB   int      `json:"total_disk_mb"`
	UsedDiskMB    int      `json:"used_disk_mb"`
}

// handleSystemResources reports the Raspberry Pi's current CPU/memory/disk
// usage. Used both to cap the instance-settings memory slider at what's
// physically available and to drive the resource-monitor panel on the
// instance list page.
func (s *Server) handleSystemResources(w http.ResponseWriter, r *http.Request) {
	cpuPercent, err := cpuUsagePercent(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalMemMB, usedMemMB, err := memoryUsageMB()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalDiskMB, usedDiskMB, err := diskUsageMB(s.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resources := systemResources{
		CPUPercent:    cpuPercent,
		CPUCount:      runtime.NumCPU(),
		TotalMemoryMB: totalMemMB,
		UsedMemoryMB:  usedMemMB,
		TotalDiskMB:   totalDiskMB,
		UsedDiskMB:    usedDiskMB,
	}
	if tempC, ok := cpuTempC(); ok {
		resources.CPUTempC = &tempC
	}
	writeJSON(w, http.StatusOK, resources)
}

// cpuStat holds the two /proc/stat "cpu" line fields we need to derive
// utilization: the sum of all time buckets and the idle bucket alone.
type cpuStat struct {
	total, idle uint64
}

// readCPUStat parses /proc/stat's aggregate "cpu" line (user, nice, system,
// idle, iowait, irq, softirq, steal, ...), all in USER_HZ jiffies since boot.
func readCPUStat() (cpuStat, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStat{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return cpuStat{}, fmt.Errorf("empty /proc/stat")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuStat{}, fmt.Errorf("unexpected /proc/stat format")
	}

	var stat cpuStat
	for i, raw := range fields[1:] {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return cpuStat{}, err
		}
		stat.total += v
		if i == 3 { // 4th bucket is idle
			stat.idle = v
		}
	}
	return stat, nil
}

// cpuUsagePercent derives instantaneous CPU utilization by sampling
// /proc/stat twice, 200ms apart, and comparing the deltas -- a single
// snapshot only gives cumulative totals since boot, not a current rate.
func cpuUsagePercent(ctx context.Context) (float64, error) {
	first, err := readCPUStat()
	if err != nil {
		return 0, err
	}
	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	second, err := readCPUStat()
	if err != nil {
		return 0, err
	}

	totalDelta := second.total - first.total
	if totalDelta == 0 {
		return 0, nil
	}
	idleDelta := second.idle - first.idle
	return 100 * float64(totalDelta-idleDelta) / float64(totalDelta), nil
}

// memoryUsageMB reads /proc/meminfo. "Used" is computed as
// MemTotal-MemAvailable rather than MemTotal-MemFree, since MemAvailable
// already accounts for reclaimable page cache/buffers that aren't actually
// under memory pressure -- MemTotal-MemFree would make a healthy, mostly-idle
// Pi look like its memory is nearly full.
func memoryUsageMB() (totalMB, usedMB int, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var totalKB, availableKB int
	found := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && found < 2 {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB, err = strconv.Atoi(fields[1])
			if err != nil {
				return 0, 0, err
			}
			found++
		case "MemAvailable:":
			availableKB, err = strconv.Atoi(fields[1])
			if err != nil {
				return 0, 0, err
			}
			found++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	return totalKB / 1024, (totalKB - availableKB) / 1024, nil
}

// cpuTempC reads the SoC temperature from the kernel's thermal sysfs
// interface (in millidegrees C). Returns ok=false, not an error, when
// unavailable -- e.g. a developer's Mac, or any non-Pi Linux box without a
// thermal_zone0 -- since this is a bonus reading the rest of the handler
// shouldn't fail over.
func cpuTempC() (float64, bool) {
	raw, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, false
	}
	milliC, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, false
	}
	return float64(milliC) / 1000, true
}

// diskUsageMB shells out to `df` rather than the raw statfs(2) syscall: the
// stdlib's syscall.Statfs_t layout differs across platforms this project's
// developers build on (e.g. macOS) but never actually runs craftdeckd on,
// whereas `df -Pk` is portable POSIX output.
func diskUsageMB(path string) (totalMB, usedMB int, err error) {
	out, err := exec.Command("df", "-Pk", path).Output()
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("unexpected df output: %q", out)
	}
	// The values are on the last line (a long filesystem name can push them
	// onto their own wrapped line on some df implementations).
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("unexpected df output: %q", out)
	}
	totalKB, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	usedKB, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, err
	}
	return totalKB / 1024, usedKB / 1024, nil
}

// handleGetSwap reports CraftDeck's own managed swap file's status --
// entirely independent of any RAM-based swap (e.g. Raspberry Pi OS's
// zram-generator) the base OS may already have running (see
// internal/swap's package doc comment).
func (s *Server) handleGetSwap(w http.ResponseWriter, r *http.Request) {
	info, err := swap.Status(r.Context(), s.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

type setSwapRequest struct {
	SizeMB int `json:"size_mb"`
}

// handleSetSwap creates CraftDeck's swap file (if it doesn't exist yet) or
// replaces it with a differently-sized one (if it does). Rejects a size
// that wouldn't leave a safety margin of free disk space rather than
// filling the disk (see swap.Set).
func (s *Server) handleSetSwap(w http.ResponseWriter, r *http.Request) {
	var req setSwapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SizeMB <= 0 {
		http.Error(w, "size_mb must be positive", http.StatusBadRequest)
		return
	}
	if err := swap.Set(r.Context(), s.dataDir, req.SizeMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	info, err := swap.Status(r.Context(), s.dataDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleDeleteSwap turns off and removes CraftDeck's swap file entirely.
func (s *Server) handleDeleteSwap(w http.ResponseWriter, r *http.Request) {
	if err := swap.Disable(r.Context(), s.dataDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
