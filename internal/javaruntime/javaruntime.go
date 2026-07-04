// Package javaruntime picks the right Java major version for a given
// Minecraft version and resolves its binary path under the Adoptium
// Temurin installs that packaging/scripts/postinst sets up (requirements.md
// FR-42, FR-42a).
package javaruntime

import (
	"fmt"
	"strconv"
	"strings"
)

// MajorForMCVersion applies requirements.md FR-42's version bands:
//   - < 1.17      -> Java 8
//   - 1.17..1.20.4 -> Java 17
//   - >= 1.20.5   -> Java 21
func MajorForMCVersion(mcVersion string) (int, error) {
	parts, err := parseVersion(mcVersion)
	if err != nil {
		return 0, err
	}
	switch {
	case compare(parts, []int{1, 17, 0}) < 0:
		return 8, nil
	case compare(parts, []int{1, 20, 5}) < 0:
		return 17, nil
	default:
		return 21, nil
	}
}

// BinaryPath returns the java executable for the given major version.
//
// ASSUMPTION: this path pattern matches Adoptium's Debian packages
// (temurin-<major>-jre) as of when this was written. Verify against the
// actual installed package layout (e.g. `dpkg -L temurin-17-jre`) before
// relying on this in production -- it has not been checked against a real
// install.
func BinaryPath(major int) string {
	return fmt.Sprintf("/usr/lib/jvm/temurin-%d-jre/bin/java", major)
}

func parseVersion(v string) ([]int, error) {
	fields := strings.Split(v, ".")
	out := make([]int, 3)
	for i := 0; i < len(fields) && i < 3; i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return nil, fmt.Errorf("parse mc_version %q: %w", v, err)
		}
		out[i] = n
	}
	return out, nil
}

func compare(a, b []int) int {
	for i := range a {
		if a[i] != b[i] {
			return a[i] - b[i]
		}
	}
	return 0
}
