package loader

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
)

const versionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type VanillaAdapter struct{}

type versionManifest struct {
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"` // "release", "snapshot", "old_beta", "old_alpha"
		URL  string `json:"url"`
	} `json:"versions"`
}

// VersionInfo is the subset of the manifest the create-instance UI needs to
// populate its Minecraft version dropdown.
type VersionInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// FetchVanillaVersions lists every Minecraft version Mojang's manifest
// currently knows about, newest first (the manifest is already ordered this
// way), so the frontend can offer a dropdown instead of a free-text version
// field an operator could typo.
func FetchVanillaVersions(ctx context.Context) ([]VersionInfo, error) {
	manifest, err := getJSON[versionManifest](ctx, versionManifestURL)
	if err != nil {
		return nil, fmt.Errorf("fetch mojang version manifest: %w", err)
	}
	versions := make([]VersionInfo, 0, len(manifest.Versions))
	for _, v := range manifest.Versions {
		versions = append(versions, VersionInfo{ID: v.ID, Type: v.Type})
	}
	return versions, nil
}

type versionMeta struct {
	Downloads struct {
		Server struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
}

func (VanillaAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	meta, err := fetchVersionMeta(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	if meta.Downloads.Server.URL == "" {
		return "", fmt.Errorf("mojang version manifest has no server download for %q", mcVersion)
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")

	if err := downloadAndVerify(ctx, meta.Downloads.Server.URL, sha1.New, meta.Downloads.Server.SHA1, jarPath); err != nil {
		return "", err
	}
	return jarPath, nil
}

func fetchVersionMeta(ctx context.Context, mcVersion string) (*versionMeta, error) {
	manifest, err := getJSON[versionManifest](ctx, versionManifestURL)
	if err != nil {
		return nil, fmt.Errorf("fetch mojang version manifest: %w", err)
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == mcVersion {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return nil, fmt.Errorf("mc_version %q not found in mojang version manifest", mcVersion)
	}

	meta, err := getJSON[versionMeta](ctx, versionURL)
	if err != nil {
		return nil, fmt.Errorf("fetch version metadata for %q: %w", mcVersion, err)
	}
	return meta, nil
}
