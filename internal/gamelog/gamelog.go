// Package gamelog manages the log files a Minecraft server/proxy already
// writes on its own via Log4j2 (<WorkDir>/logs/latest.log plus rotated
// <date>-<n>.log.gz files) -- every loader CraftDeck supports (Vanilla,
// every Paper-family fork, Fabric, NeoForge) already does this on its own;
// CraftDeck never touches log4j2 configuration and doesn't need to. What
// none of them do is ever clean up after themselves, so this package adds
// an operator-configurable retention policy (see Settings) on top, plus a
// ReadHistory helper the console's "scroll up for more" feature uses to
// page back through those same files instead of maintaining a separate,
// redundant copy of everything the game is already logging.
package gamelog

import (
	"bufio"
	"compress/gzip"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Settings mirrors the operator-facing per-instance log-retention controls
// (see instance.Instance's LogStorage* fields).
type Settings struct {
	Enabled        bool
	RetentionMode  string // "unlimited" | "age" | "size"
	RetentionDays  int
	RetentionMaxMB int
}

// activeLogNames are the files Log4j2 (or NeoForge's extra debug appender)
// is actively writing to right now -- never candidates for deletion
// regardless of retention settings, since removing one out from under the
// still-running JVM would either fail outright or just get silently
// recreated, and either way isn't what "clean up old logs" is supposed to
// mean.
var activeLogNames = map[string]bool{
	"latest.log": true,
	"debug.log":  true, // NeoForge/Forge's extra debug-level stream
}

// rotatedNameRE matches Log4j2's default rotation filename, shared by every
// loader CraftDeck supports (Vanilla/Paper-family/Fabric all use it as-is;
// NeoForge's debug-N.log.gz uses a plain incrementing counter instead of a
// date and is handled separately in listLogFiles).
var rotatedNameRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-\d+\.log\.gz$`)

var debugRotatedNameRE = regexp.MustCompile(`^debug-\d+\.log\.gz$`)

// EnforceRetention deletes rotated (non-active) log files in logsDir per
// settings.RetentionMode. Call this after each canary-style checkpoint --
// on instance start, right after an operator changes the setting, and once
// a day from a background sweep (see cmd/craftdeckd/main.go) to catch a
// long-running instance that rotates mid-session without ever restarting.
func EnforceRetention(logsDir string, settings Settings) {
	if !settings.Enabled || settings.RetentionMode == "unlimited" {
		return
	}
	files, err := listLogFiles(logsDir)
	if err != nil {
		log.Printf("gamelog: list log files in %s: %v", logsDir, err)
		return
	}

	switch settings.RetentionMode {
	case "age":
		days := settings.RetentionDays
		if days <= 0 {
			days = 30
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		for _, f := range files {
			if f.modTime.Before(cutoff) {
				if err := os.Remove(f.path); err != nil {
					log.Printf("gamelog: remove old log %s: %v", f.path, err)
				}
			}
		}
	case "size":
		maxBytes := int64(settings.RetentionMaxMB) * 1024 * 1024
		if maxBytes <= 0 {
			maxBytes = 500 * 1024 * 1024
		}
		sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) }) // oldest first
		var total int64
		for _, f := range files {
			total += f.size
		}
		for _, f := range files {
			if total <= maxBytes {
				break
			}
			if err := os.Remove(f.path); err != nil {
				log.Printf("gamelog: remove old log %s: %v", f.path, err)
				continue
			}
			total -= f.size
		}
	}
}

type logFile struct {
	path    string
	name    string
	modTime time.Time
	size    int64
}

// listLogFiles returns every *rotated* log file in logsDir -- latest.log
// and debug.log (see activeLogNames) are deliberately excluded, since
// they're always in use and never eligible for retention/history walking
// the same way a finished, rotated file is.
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
		if activeLogNames[name] {
			continue
		}
		if !rotatedNameRE.MatchString(name) && !debugRotatedNameRE.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			path:    filepath.Join(logsDir, name),
			name:    name,
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}
	return files, nil
}

// Entry is one parsed line from the server's own Log4j2 output, with the
// timestamp ReadHistory reconstructed for it (see parseLines).
type Entry struct {
	At   time.Time
	Line string
}

var linePrefixRE = regexp.MustCompile(`^\[(\d{2}):(\d{2}):(\d{2})\]`)

// ReadHistory returns up to limit entries strictly older than before, in
// chronological order (oldest of the batch first, ready to prepend
// directly to whatever the console already has loaded), plus whether
// still-older entries exist beyond what was returned. Reads logs/latest.log
// plus however many rotated *.log.gz files (in reverse date order) are
// needed -- NeoForge's separate debug-*.log.gz stream is deliberately
// excluded, since it's a much noisier, differently-scoped log not meant
// for the console view.
func ReadHistory(logsDir string, before time.Time, limit int) ([]Entry, bool, error) {
	sources, err := historySources(logsDir)
	if err != nil {
		return nil, false, err
	}

	var out []Entry // collected newest-to-oldest below; reversed just before returning
	hasMore := false
outer:
	for _, src := range sources {
		lines, err := readLines(src.path)
		if err != nil {
			log.Printf("gamelog: read %s: %v (skipping)", src.path, err)
			continue
		}
		entries := parseLines(lines, src.date)
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

type historySource struct {
	path string
	date time.Time // the calendar day this file's lines belong to
}

// historySources lists latest.log (today, if present) followed by every
// non-debug rotated file newest-date-first, for ReadHistory to walk
// backwards through.
func historySources(logsDir string) ([]historySource, error) {
	var sources []historySource
	if info, err := os.Stat(filepath.Join(logsDir, "latest.log")); err == nil && !info.IsDir() {
		// Log4j2 rotates at local midnight (and on restart), so latest.log
		// always covers "today" in any normally-configured setup -- not
		// worth tracking exactly when the last rotation happened just to
		// handle the edge case of a server that's been up for weeks with a
		// broken rotation config.
		sources = append(sources, historySource{path: filepath.Join(logsDir, "latest.log"), date: time.Now()})
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return sources, nil
		}
		return nil, err
	}
	var rotated []historySource
	for _, e := range entries {
		name := e.Name()
		m := rotatedNameRE.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", m[1], time.Local)
		if err != nil {
			continue
		}
		rotated = append(rotated, historySource{path: filepath.Join(logsDir, name), date: date})
	}
	sort.Slice(rotated, func(i, j int) bool { return rotated[i].date.After(rotated[j].date) })
	return append(sources, rotated...), nil
}

// readLines returns path's lines, transparently gunzipping if it ends in
// .gz.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}

	var out []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	return out, scanner.Err()
}

// parseLines turns raw Log4j2 lines into timestamped Entry values. Every
// loader CraftDeck supports uses the same "[HH:MM:SS] [thread/LEVEL]: msg"
// prefix (confirmed: none of them touch log4j2's format), so a shared
// regex covers all of them. A line that doesn't start with a recognizable
// time (a wrapped stack trace, a multi-line exception, ...) inherits the
// previous line's timestamp instead of being dropped, so it still shows up
// attached to roughly the right place instead of vanishing.
func parseLines(lines []string, date time.Time) []Entry {
	out := make([]Entry, 0, len(lines))
	last := date
	for _, line := range lines {
		at := last
		if m := linePrefixRE.FindStringSubmatch(line); m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			s, _ := strconv.Atoi(m[3])
			at = time.Date(date.Year(), date.Month(), date.Day(), h, mi, s, 0, date.Location())
			last = at
		}
		out = append(out, Entry{At: at, Line: line})
	}
	return out
}
