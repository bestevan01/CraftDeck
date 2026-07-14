package loader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// fillProjectVersions lists every version a PaperMC "fill" API v3 project
// (paper, velocity, ...) currently publishes builds for, newest first. The
// API groups versions (e.g. Paper by Minecraft minor release, Velocity by
// its own major.minor), with both the groups and each group's entries
// already ordered newest-first -- preserved here by walking the raw JSON
// object's keys in their original order, since decoding straight into a Go
// map would lose that ordering.
func fillProjectVersions(ctx context.Context, apiBase string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch project versions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, apiBase)
	}

	var body struct {
		Versions json.RawMessage `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode project versions: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(body.Versions))
	if _, err := dec.Token(); err != nil { // consume opening '{'
		return nil, err
	}
	var versions []string
	for dec.More() {
		if _, err := dec.Token(); err != nil { // group name key, unused
			return nil, err
		}
		var group []string
		if err := dec.Decode(&group); err != nil {
			return nil, err
		}
		versions = append(versions, group...)
	}
	return versions, nil
}

// fillLatestBuildableVersion returns the best entry in fillProjectVersions'
// newest-first list to auto-install: the newest *proper release* (no
// "-SNAPSHOT"/"-beta"/etc suffix) that actually has at least one published
// build, skipping both unbuilt and pre-release entries. Two real gaps in
// just trusting index 0 motivated this, both confirmed against Velocity's
// project: (1) the fill API can list a version before any build exists for
// it -- "4.0.0" got its own version group the moment it was announced but
// had zero builds; (2) even once a pre-release branch of that new major
// version does have builds ("4.0.0-SNAPSHOT"), it's a bigger jump than
// necessary when the actual problem (e.g. missing support for a new
// Minecraft protocol version) is often already fixed in the newest proper
// release of the current major line ("3.5.1"), which is also far less
// likely to need a config-format migration CraftDeck doesn't generate yet.
func fillLatestBuildableVersion(ctx context.Context, apiBase string) (string, error) {
	versions, err := fillProjectVersions(ctx, apiBase)
	if err != nil {
		return "", err
	}

	hasBuilds := func(v string) bool {
		builds, err := getJSON[[]fillBuild](ctx, fmt.Sprintf("%s/versions/%s/builds", apiBase, v))
		return err == nil && len(*builds) > 0
	}

	for _, v := range versions {
		if !strings.Contains(v, "-") && hasBuilds(v) {
			return v, nil
		}
	}
	// Nothing shipped as a clean release yet; fall back to the newest
	// pre-release build rather than failing outright.
	for _, v := range versions {
		if hasBuilds(v) {
			return v, nil
		}
	}
	return "", fmt.Errorf("no buildable version found under %s", apiBase)
}

type fillBuild struct {
	ID        int    `json:"id"`
	Channel   string `json:"channel"` // "STABLE", "BETA", or "ALPHA"
	Time      string `json:"time"`
	Downloads struct {
		ServerDefault struct {
			Name      string `json:"name"`
			Checksums struct {
				SHA256 string `json:"sha256"`
			} `json:"checksums"`
			URL string `json:"url"`
		} `json:"server:default"`
	} `json:"downloads"`
}

// fillDownloadBuild downloads+verifies one fillBuild's server jar into
// destDir/server.jar -- the shared tail end of fillDownload and
// fillDownloadBuild (the BuildLister method), which only differ in how
// they pick *which* build to pass in here.
func fillDownloadBuildJar(ctx context.Context, build fillBuild, version, destDir string) (string, error) {
	if build.Downloads.ServerDefault.URL == "" {
		return "", fmt.Errorf("build %d for %q has no server download", build.ID, version)
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")
	if err := downloadAndVerify(
		ctx, build.Downloads.ServerDefault.URL, sha256.New,
		build.Downloads.ServerDefault.Checksums.SHA256, jarPath,
	); err != nil {
		return "", err
	}
	return jarPath, nil
}

// fillDownload fetches apiBase's newest STABLE build for version (falling
// back to the newest build of any channel if that version has no stable
// build yet) and downloads+verifies its server jar into destDir/server.jar.
func fillDownload(ctx context.Context, apiBase, version, destDir string) (string, error) {
	builds, err := getJSON[[]fillBuild](ctx, fmt.Sprintf("%s/versions/%s/builds", apiBase, version))
	if err != nil {
		return "", fmt.Errorf("fetch builds for %q: %w", version, err)
	}
	if len(*builds) == 0 {
		return "", fmt.Errorf("no builds found for version %q", version)
	}

	// Builds are listed newest-first; pick the first STABLE one.
	build := (*builds)[0]
	for _, b := range *builds {
		if b.Channel == "STABLE" {
			build = b
			break
		}
	}
	return fillDownloadBuildJar(ctx, build, version, destDir)
}

// fillListBuilds lists version's builds newest-first, for BuildLister.
func fillListBuilds(ctx context.Context, apiBase, version string) ([]BuildInfo, error) {
	builds, err := getJSON[[]fillBuild](ctx, fmt.Sprintf("%s/versions/%s/builds", apiBase, version))
	if err != nil {
		return nil, fmt.Errorf("fetch builds for %q: %w", version, err)
	}
	out := make([]BuildInfo, len(*builds))
	for i, b := range *builds {
		out[i] = BuildInfo{ID: strconv.Itoa(b.ID), Channel: b.Channel, Time: b.Time}
	}
	return out, nil
}

// fillDownloadBuild downloads version's build buildID specifically, for
// BuildLister.
func fillDownloadBuild(ctx context.Context, apiBase, version, buildID, destDir string) (string, error) {
	builds, err := getJSON[[]fillBuild](ctx, fmt.Sprintf("%s/versions/%s/builds", apiBase, version))
	if err != nil {
		return "", fmt.Errorf("fetch builds for %q: %w", version, err)
	}
	for _, b := range *builds {
		if strconv.Itoa(b.ID) == buildID {
			return fillDownloadBuildJar(ctx, b, version, destDir)
		}
	}
	return "", fmt.Errorf("build %q not found for version %q", buildID, version)
}
