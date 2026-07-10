// Package loader downloads Minecraft server jars from each loader's
// official distribution point (requirements.md FR-1, FR-2). Vanilla and
// Paper adapters are implemented so far; Purpur/Fabric/Forge/Velocity/
// BungeeCord adapters follow the same Adapter interface.
package loader

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
)

// Adapter fetches one loader's server jar for a given Minecraft version and
// writes it to destDir/server.jar.
type Adapter interface {
	// Download places the server jar at destDir/server.jar, returning its
	// path. mcVersion is the Minecraft version string (e.g. "1.21").
	Download(ctx context.Context, mcVersion string, destDir string) (jarPath string, err error)
}

var registry = map[string]Adapter{
	"vanilla":  VanillaAdapter{},
	"paper":    PaperAdapter{},
	"velocity": VelocityAdapter{},
}

func Get(loaderName string) (Adapter, bool) {
	a, ok := registry[loaderName]
	return a, ok
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

// downloadAndVerify streams url to destPath, optionally verifying its
// checksum against expectedHex. newHash selects the algorithm (sha1.New for
// Mojang's manifest, sha256.New for PaperMC's API); pass nil to skip
// verification entirely (see requirements.md FR-6d for the analogous
// Modrinth hash-verification requirement on the plugin/mod side).
func downloadAndVerify(ctx context.Context, url string, newHash func() hash.Hash, expectedHex, destPath string) error {
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

	var hasher hash.Hash
	dest := io.Writer(f)
	if newHash != nil {
		hasher = newHash()
		dest = io.MultiWriter(f, hasher)
	}
	if _, err := io.Copy(dest, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if hasher != nil && expectedHex != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if got != expectedHex {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum mismatch for %s: got %s, want %s", destPath, got, expectedHex)
		}
	}

	return os.Rename(tmpPath, destPath)
}
