// Package gamelog captures each running instance's live console output to
// its own on-disk log files -- independent of journald's system-wide (not
// per-instance) retention, and of whether anyone has the console tab open
// at the time a line is emitted -- so the console's "load more" history in
// the UI can go back arbitrarily far instead of whatever `journalctl -n 50`
// happens to still have.
package gamelog

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Settings mirrors the operator-facing per-instance log-storage controls
// (see instance.Instance's LogStorage* fields) -- passed in rather than
// read from the DB directly so the capture loop doesn't need its own DB
// handle.
type Settings struct {
	Enabled        bool
	RetentionMode  string // "unlimited" | "age" | "size"
	RetentionDays  int
	RetentionMaxMB int
}

// Manager runs one background journalctl-tail-to-file goroutine per
// currently-capturing instance.
type Manager struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewManager() *Manager {
	return &Manager{cancels: make(map[string]context.CancelFunc)}
}

// StartCapturing begins (or restarts) capturing unit's journal output to
// logsDir for instanceID, per settings. Call this whenever an instance
// starts (and once at daemon startup for whatever's already running -- see
// cmd/craftdeckd/main.go's reconcileInstances) and again whenever its log
// settings change while running, so a toggle takes effect immediately
// instead of needing a restart. Settings.Enabled == false just stops any
// existing capture.
func (m *Manager) StartCapturing(instanceID, unit, logsDir string, settings Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, ok := m.cancels[instanceID]; ok {
		cancel()
		delete(m.cancels, instanceID)
	}
	if !settings.Enabled {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[instanceID] = cancel
	go capture(ctx, unit, logsDir, settings)
}

// StopCapturing stops instanceID's capture goroutine, if any. Call this
// when the instance stops or is deleted.
func (m *Manager) StopCapturing(instanceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.cancels[instanceID]; ok {
		cancel()
		delete(m.cancels, instanceID)
	}
}

const filenameLayout = "2006-01-02" // one file per calendar day, local time

func capture(ctx context.Context, unit, logsDir string, settings Settings) {
	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		log.Printf("gamelog %s: create logs dir: %v", unit, err)
		return
	}

	// -n 0: only lines emitted from here on. The one caller of
	// StartCapturing (startInstanceCore) only fires once the process is
	// (about to be) running, so there's no gap to backfill, and re-sending
	// old lines here would just duplicate whatever a previous capture run
	// already wrote to today's file.
	cmd := exec.CommandContext(ctx, "journalctl", "-u", unit, "-f", "-n", "0", "-o", "cat")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("gamelog %s: stdout pipe: %v", unit, err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("gamelog %s: start journalctl: %v", unit, err)
		return
	}
	defer cmd.Wait() //nolint:errcheck // killed via ctx cancellation on return

	enforceRetention(logsDir, settings) // sweep on every (re)start, not just rotation

	stopTicker := make(chan struct{})
	defer close(stopTicker)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopTicker:
				return
			case <-ticker.C:
				enforceRetention(logsDir, settings)
			}
		}
	}()

	var file *os.File
	var fileDate string
	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		now := time.Now()
		date := now.Format(filenameLayout)
		if date != fileDate {
			if file != nil {
				file.Close()
				file = nil
			}
			f, err := os.OpenFile(filepath.Join(logsDir, "console-"+date+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
			if err != nil {
				log.Printf("gamelog %s: open log file: %v (dropping lines until next rotation)", unit, err)
				fileDate = date
				continue
			}
			file = f
			fileDate = date
			enforceRetention(logsDir, settings)
		}
		if file == nil {
			continue
		}
		// "<RFC3339Nano timestamp>\t<raw line>\n" -- the timestamp is our
		// own receive time, not journald's: -f is a live tail so the lag is
		// negligible, and this avoids having to parse journald's own
		// timestamp format back out later. Stripped again before a line
		// ever reaches the frontend (see ReadHistory) so a stored line
		// renders identically to how it looked live.
		fmt.Fprintf(file, "%s\t%s\n", now.Format(time.RFC3339Nano), scanner.Text())
	}
}

type logFile struct {
	path string
	date string
	size int64
}

func listLogFiles(logsDir string) ([]logFile, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []logFile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "console-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			path: filepath.Join(logsDir, name),
			date: strings.TrimSuffix(strings.TrimPrefix(name, "console-"), ".log"),
			size: info.Size(),
		})
	}
	return files, nil
}

// enforceRetention deletes old log files per settings.RetentionMode. Never
// deletes today's file even if size/age accounting would otherwise call
// for it -- there's always exactly one file actively being appended to,
// and the point is bounding history, not breaking the capture that's
// currently running.
func enforceRetention(logsDir string, settings Settings) {
	if settings.RetentionMode == "unlimited" {
		return
	}
	files, err := listLogFiles(logsDir)
	if err != nil {
		log.Printf("gamelog: list log files in %s: %v", logsDir, err)
		return
	}
	today := time.Now().Format(filenameLayout)

	switch settings.RetentionMode {
	case "age":
		days := settings.RetentionDays
		if days <= 0 {
			days = 30
		}
		cutoff := time.Now().AddDate(0, 0, -days).Format(filenameLayout)
		for _, f := range files {
			if f.date == today || f.date >= cutoff {
				continue
			}
			if err := os.Remove(f.path); err != nil {
				log.Printf("gamelog: remove old log %s: %v", f.path, err)
			}
		}
	case "size":
		maxBytes := int64(settings.RetentionMaxMB) * 1024 * 1024
		if maxBytes <= 0 {
			maxBytes = 500 * 1024 * 1024
		}
		sort.Slice(files, func(i, j int) bool { return files[i].date < files[j].date }) // oldest first
		var total int64
		for _, f := range files {
			total += f.size
		}
		for _, f := range files {
			if total <= maxBytes {
				break
			}
			if f.date == today {
				continue
			}
			if err := os.Remove(f.path); err != nil {
				log.Printf("gamelog: remove old log %s: %v", f.path, err)
				continue
			}
			total -= f.size
		}
	}
}

// Entry is one stored console line with the timestamp it was captured at.
// Line is exactly the raw text the live WebSocket console would have sent
// for the same moment -- the storage-only timestamp prefix is stripped
// before it gets here.
type Entry struct {
	At   time.Time
	Line string
}

// ReadHistory returns up to limit entries strictly older than before, in
// chronological order (oldest of the batch first, matching how the
// frontend's own in-memory line buffer is ordered so a batch can just be
// prepended as-is), plus whether still-older entries exist beyond what was
// returned. Reads whichever daily files are needed, newest to oldest,
// stopping as soon as limit is reached.
func ReadHistory(logsDir string, before time.Time, limit int) ([]Entry, bool, error) {
	files, err := listLogFiles(logsDir)
	if err != nil {
		return nil, false, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].date > files[j].date }) // newest first

	var out []Entry // collected newest-to-oldest below; reversed just before returning
	hasMore := false
outer:
	for _, f := range files {
		entries, err := readFileEntries(f.path)
		if err != nil {
			log.Printf("gamelog: read %s: %v (skipping)", f.path, err)
			continue
		}
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if !e.At.Before(before) {
				continue
			}
			if len(out) >= limit {
				hasMore = true
				break outer
			}
			out = append(out, e)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore, nil
}

func readFileEntries(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, line[:tab])
		if err != nil {
			continue
		}
		out = append(out, Entry{At: at, Line: line[tab+1:]})
	}
	return out, scanner.Err()
}
