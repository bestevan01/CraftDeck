package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// systemResources is the payload for GET /api/system/resources: covers both
// the memory cap the instance-settings slider needs and the live usage
// numbers shown on the resource-monitor panel of the instance list page.
type systemResources struct {
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCount      int     `json:"cpu_count"`
	TotalMemoryMB int     `json:"total_memory_mb"`
	UsedMemoryMB  int     `json:"used_memory_mb"`
	TotalDiskMB   int     `json:"total_disk_mb"`
	UsedDiskMB    int     `json:"used_disk_mb"`
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

	writeJSON(w, http.StatusOK, systemResources{
		CPUPercent:    cpuPercent,
		CPUCount:      runtime.NumCPU(),
		TotalMemoryMB: totalMemMB,
		UsedMemoryMB:  usedMemMB,
		TotalDiskMB:   totalDiskMB,
		UsedDiskMB:    usedDiskMB,
	})
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
