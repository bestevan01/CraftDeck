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

type fillBuild struct {
	ID        int    `json:"id"`
	Channel   string `json:"channel"` // "STABLE", "BETA", or "ALPHA"
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
