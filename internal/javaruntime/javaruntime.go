// Package javaruntime picks the right Java major version for a given
// Minecraft version and resolves its binary path under the Adoptium
// Temurin installs that packaging/scripts/postinst sets up (requirements.md
// FR-42, FR-42a).
package javaruntime

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// MajorForMCVersion applies requirements.md FR-42's version bands:
//   - < 1.17       -> Java 8
//   - 1.17..1.20.4 -> Java 17
//   - 1.20.5..26.0 -> Java 21
//   - >= 26.1      -> Java 25 (FR-42g: Mojang switched to a year.release
//     version scheme starting with 26.1, and recommends Java 25 from that
//     version on)
func MajorForMCVersion(mcVersion string) (int, error) {
	// Versions under the old "1.x[.y]" scheme all start with "1."; anything
	// else is the new year.release scheme (e.g. "26.1", "26.2"), which is
	// unambiguously >= 26.1 since that's where the new scheme started, so it
	// always maps to Java 25.
	if !strings.HasPrefix(mcVersion, "1.") {
		return 25, nil
	}

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

// installedMajors are the Temurin JREs packaging/scripts/postinst
// provisions, oldest first -- kept in sync with that list by hand rather
// than probed at runtime, since nothing else in this codebase queries
// installed JVMs dynamically either.
var installedMajors = []int{8, 17, 21, 25}

// NearestInstalledMajor returns the smallest installed Java major that
// satisfies a "minimum required" version (e.g. Velocity's own fill-API
// metadata, see loader.FetchVelocityJavaMinimum) -- falls back to the
// newest installed major if even that isn't enough, on the theory that
// trying the newest available beats refusing to start at all.
func NearestInstalledMajor(minimum int) int {
	for _, m := range installedMajors {
		if m >= minimum {
			return m
		}
	}
	return installedMajors[len(installedMajors)-1]
}

// BinaryPath returns the java executable for the given major version.
//
// VERIFIED against a real install: `apt-get install temurin-21-jre` on
// Raspberry Pi OS (Debian 13/trixie, arm64) places binaries under
// /usr/lib/jvm/temurin-<major>-jre-<debian-arch>/bin/java -- note the
// architecture suffix, which an earlier unverified version of this function
// omitted.
func BinaryPath(major int) string {
	return fmt.Sprintf("/usr/lib/jvm/temurin-%d-jre-%s/bin/java", major, debianArch())
}

// debianArch maps Go's GOARCH to the Debian architecture suffix Adoptium's
// packages use.
func debianArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "arm":
		return "armhf" // Raspberry Pi OS's 32-bit builds are hard-float
	default:
		return runtime.GOARCH
	}
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
