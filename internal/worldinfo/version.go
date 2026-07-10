package worldinfo

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// DetectVersionFromLevelDat reads a world's level.dat (standard
// gzip-compressed NBT) and returns the Minecraft version string recorded at
// Data.Version.Name (present since 1.9), falling back to a formatted
// data-version Id if Name is absent.
func DetectVersionFromLevelDat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return detectVersion(f)
}

// DetectVersionFromArchive looks inside a gzip-compressed tar archive (as
// produced by internal/backup) for the first "level.dat" entry -- whatever
// depth it's nested at, since the archive root is a work directory
// containing "<level-name>/level.dat" -- and detects its version the same
// way as DetectVersionFromLevelDat.
func DetectVersionFromArchive(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("no level.dat found in archive")
		}
		if err != nil {
			return "", err
		}
		if header.Name == "level.dat" || strings.HasSuffix(header.Name, "/level.dat") {
			return detectVersion(tr)
		}
	}
}

func detectVersion(r io.Reader) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("level.dat is not gzip-compressed NBT: %w", err)
	}
	defer gz.Close()

	root, err := decodeRootCompound(gz)
	if err != nil {
		return "", fmt.Errorf("parse level.dat NBT: %w", err)
	}

	data, ok := root["Data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("level.dat has no Data compound")
	}
	version, ok := data["Version"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("world predates the Version tag (pre-1.9) -- version can't be determined")
	}
	if name, ok := version["Name"].(string); ok && name != "" {
		return name, nil
	}
	if id, ok := version["Id"].(int32); ok {
		return fmt.Sprintf("data version %d", id), nil
	}
	return "", fmt.Errorf("level.dat's Version tag has neither Name nor Id")
}

// CompareClassicVersions reports whether version a is strictly newer than b,
// for the classic "1.x[.y]" scheme only (Minecraft versions before the
// year.release scheme introduced at 26.1 -- see internal/javaruntime).
// comparable is false if either string doesn't parse as that scheme, in
// which case callers should skip strict validation rather than guess.
func CompareClassicVersions(a, b string) (newer bool, comparable bool) {
	pa, errA := parseClassicVersion(a)
	pb, errB := parseClassicVersion(b)
	if errA != nil || errB != nil {
		return false, false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i], true
		}
	}
	return false, true
}

func parseClassicVersion(v string) ([3]int, error) {
	if !strings.HasPrefix(v, "1.") {
		return [3]int{}, fmt.Errorf("not classic 1.x version scheme")
	}
	fields := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < len(fields) && i < 3; i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return [3]int{}, fmt.Errorf("parse version %q: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}
