package loader

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const versionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

type VanillaAdapter struct{}

type versionManifest struct {
	Versions []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"versions"`
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

	if err := downloadAndVerify(ctx, meta.Downloads.Server.URL, meta.Downloads.Server.SHA1, jarPath); err != nil {
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

func getJSON[T any](ctx context.Context, url string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// downloadAndVerify streams url to destPath, verifying its SHA-1 against
// expectedSHA1 (Mojang publishes this; see requirements.md FR-6d for the
// analogous Modrinth hash-verification requirement on the plugin/mod side).
func downloadAndVerify(ctx context.Context, url, expectedSHA1, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, url)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	hasher := sha1.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if expectedSHA1 != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != expectedSHA1 {
			os.Remove(tmpPath)
			return fmt.Errorf("sha1 mismatch for %s: got %s, want %s", destPath, got, expectedSHA1)
		}
	}

	return os.Rename(tmpPath, destPath)
}
