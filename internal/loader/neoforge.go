package loader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"craftdeck/internal/javaruntime"
)

const (
	neoForgeMavenVersionsURL = "https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge"
	neoForgeInstallerURLFmt  = "https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar"
	// serverStarterJarURL is NeoForged's official same-process wrapper
	// around the Forge(1.17+)/NeoForge run scripts -- it brings back a
	// plain "server.jar" you invoke with "java -jar server.jar", matching
	// every other loader CraftDeck supports, instead of the argument-file
	// launch (@user_jvm_args.txt @libraries/.../unix_args.txt) those run
	// scripts otherwise require.
	serverStarterJarURL = "https://github.com/NeoForged/ServerStarterJar/releases/latest/download/server.jar"
)

type NeoForgeAdapter struct{}

type neoForgeVersionsResponse struct {
	Versions []string `json:"versions"`
}

// classicNeoForgeVersionRE matches NeoForge's "1.x"-era version scheme:
// <mc-minor>.<mc-patch>.<build>, optionally suffixed "-beta"/"-alpha" (e.g.
// "21.1.235", "20.6.139-beta"). Minecraft's newer year.release scheme
// (26.x) maps to a different, still-evolving NeoForge scheme with only a
// couple of experimental builds published so far, so it's out of scope for
// now (see mcVersionToNeoForgePrefix).
var classicNeoForgeVersionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(-beta|-alpha)?$`)

// mcVersionToNeoForgePrefix converts a "1.x[.y]" Minecraft version into the
// (minor, patch) NeoForge uses as its own version's leading two numbers
// (e.g. "1.21.1" -> ("21","1"), "1.21" -> ("21","0") -- NeoForge always
// writes a ".0" patch placeholder for a bare minor release). Returns ""
// for anything outside the classic scheme.
func mcVersionToNeoForgePrefix(mcVersion string) (minor, patch string) {
	if !strings.HasPrefix(mcVersion, "1.") {
		return "", ""
	}
	rest := strings.TrimPrefix(mcVersion, "1.")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "0"
}

// neoForgeMCVersion is mcVersionToNeoForgePrefix's inverse.
func neoForgeMCVersion(minor, patch string) string {
	if patch == "0" {
		return "1." + minor
	}
	return "1." + minor + "." + patch
}

// FetchNeoForgeVersions lists every Minecraft version NeoForge has a
// released build for, newest first, derived from NeoForge's own version
// list since it doesn't publish a Minecraft-version-keyed index the way
// Paper's fill API does.
func FetchNeoForgeVersions(ctx context.Context) ([]string, error) {
	resp, err := getJSON[neoForgeVersionsResponse](ctx, neoForgeMavenVersionsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch neoforge versions: %w", err)
	}

	seen := map[string]bool{}
	var mcVersions []string
	for _, v := range resp.Versions {
		m := classicNeoForgeVersionRE.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		mc := neoForgeMCVersion(m[1], m[2])
		if !seen[mc] {
			seen[mc] = true
			mcVersions = append(mcVersions, mc)
		}
	}
	sort.Slice(mcVersions, func(i, j int) bool { return compareVersions(mcVersions[i], mcVersions[j]) > 0 })
	return mcVersions, nil
}

// neoForgeVersionsForMC lists every NeoForge version string targeting
// mcVersion, newest build first -- the shared lookup behind
// latestNeoForgeVersion and ListBuilds (FR-4).
func neoForgeVersionsForMC(ctx context.Context, mcVersion string) ([]string, error) {
	resp, err := getJSON[neoForgeVersionsResponse](ctx, neoForgeMavenVersionsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch neoforge versions: %w", err)
	}
	minor, patch := mcVersionToNeoForgePrefix(mcVersion)
	if minor == "" {
		return nil, fmt.Errorf("neoforge doesn't support minecraft version %q yet", mcVersion)
	}

	type match struct {
		version string
		build   int
	}
	var matches []match
	for _, v := range resp.Versions {
		m := classicNeoForgeVersionRE.FindStringSubmatch(v)
		if m == nil || m[1] != minor || m[2] != patch {
			continue
		}
		build, _ := strconv.Atoi(m[3])
		matches = append(matches, match{version: v, build: build})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].build > matches[j].build })
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.version
	}
	return out, nil
}

// latestNeoForgeVersion finds the newest NeoForge build targeting
// mcVersion, preferring a non-beta/alpha release but falling back to a
// pre-release build if that's all that exists yet for a very recent
// Minecraft version.
func latestNeoForgeVersion(ctx context.Context, mcVersion string) (string, error) {
	versions, err := neoForgeVersionsForMC(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	for _, v := range versions { // newest-build-first already
		if !strings.Contains(v, "-") { // no "-beta"/"-alpha" suffix
			return v, nil
		}
	}
	if len(versions) > 0 {
		return versions[0], nil
	}
	return "", fmt.Errorf("no neoforge build found for minecraft version %q", mcVersion)
}

// installNeoForgeVersion downloads the NeoForge installer for the exact
// neoVersion given and runs it (--installServer) synchronously to lay down
// run.sh/libraries, then places ServerStarterJar at destDir/server.jar so
// the instance can be launched the same "java -jar server.jar" way as
// every other loader. Shared by Download (resolves neoVersion itself) and
// DownloadBuild (BuildLister, given an exact version to pin).
//
// Running the installer here (during provisioning, not under systemd) is
// safe because --installServer only extracts libraries and writes the run
// scripts -- unlike actually executing run.sh afterward, it never boots a
// live Minecraft server (that first real boot, which needs eula.txt
// already accepted, happens later when the operator starts the instance).
func installNeoForgeVersion(ctx context.Context, neoVersion, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}

	installerPath := filepath.Join(destDir, "neoforge-installer.jar")
	installerURL := fmt.Sprintf(neoForgeInstallerURLFmt, neoVersion, neoVersion)
	if err := downloadAndVerify(ctx, installerURL, nil, "", installerPath); err != nil {
		return "", fmt.Errorf("download neoforge installer: %w", err)
	}
	defer os.Remove(installerPath)

	m := classicNeoForgeVersionRE.FindStringSubmatch(neoVersion)
	if m == nil {
		return "", fmt.Errorf("invalid neoforge version %q", neoVersion)
	}
	mcVersion := neoForgeMCVersion(m[1], m[2])
	javaMajor, err := javaruntime.MajorForMCVersion(mcVersion)
	if err != nil {
		return "", fmt.Errorf("determine java runtime for %q: %w", mcVersion, err)
	}

	cmd := exec.CommandContext(ctx, javaruntime.BinaryPath(javaMajor), "-jar", installerPath, "--installServer")
	cmd.Dir = destDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("run neoforge installer: %w\n%s", err, output)
	}

	jarPath := filepath.Join(destDir, "server.jar")
	if err := downloadAndVerify(ctx, serverStarterJarURL, nil, "", jarPath); err != nil {
		return "", fmt.Errorf("download ServerStarterJar: %w", err)
	}
	return jarPath, nil
}

func (NeoForgeAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	neoVersion, err := latestNeoForgeVersion(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	return installNeoForgeVersion(ctx, neoVersion, destDir)
}

// ListBuilds/DownloadBuild implement BuildLister (FR-4): each NeoForge
// version string targeting mcVersion is treated as one "build" -- there's
// no separate build-number concept beyond the version string itself.
func (NeoForgeAdapter) ListBuilds(ctx context.Context, mcVersion string) ([]BuildInfo, error) {
	versions, err := neoForgeVersionsForMC(ctx, mcVersion)
	if err != nil {
		return nil, err
	}
	out := make([]BuildInfo, len(versions))
	for i, v := range versions {
		channel := "STABLE"
		if strings.Contains(v, "-") {
			channel = "BETA"
		}
		out[i] = BuildInfo{ID: v, Channel: channel}
	}
	return out, nil
}

func (NeoForgeAdapter) DownloadBuild(ctx context.Context, mcVersion, buildID, destDir string) (string, error) {
	versions, err := neoForgeVersionsForMC(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	for _, v := range versions {
		if v == buildID {
			return installNeoForgeVersion(ctx, v, destDir)
		}
	}
	return "", fmt.Errorf("neoforge version %q not found for minecraft version %q", buildID, mcVersion)
}
