package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// fabricMetaBase is Fabric's official distribution API. Unlike the Paper-
// family loaders, a ready-to-run server jar isn't addressed by Minecraft
// version alone -- it's a combination of (game version, loader version,
// installer version), so FetchFabricVersions/Download always resolve the
// latest stable loader+installer at request time rather than storing them
// per instance. The resulting jar bootstraps the actual vanilla server +
// Fabric loader itself on first launch (needs network access then), but
// runs with the same "java -jar server.jar" invocation as every other
// loader CraftDeck supports.
const fabricMetaBase = "https://meta.fabricmc.net/v2"

type FabricAdapter struct{}

type fabricGameVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// FetchFabricVersions lists every Minecraft version Fabric's intermediary
// mappings mark stable, newest first (matches Vanilla's release-only
// filtering -- Fabric's own API also lists snapshots, which aren't useful
// to offer in the create-instance dropdown).
func FetchFabricVersions(ctx context.Context) ([]string, error) {
	all, err := getJSON[[]fabricGameVersion](ctx, fabricMetaBase+"/versions/game")
	if err != nil {
		return nil, fmt.Errorf("fetch fabric game versions: %w", err)
	}
	versions := make([]string, 0, len(*all))
	for _, v := range *all {
		if v.Stable {
			versions = append(versions, v.Version)
		}
	}
	return versions, nil
}

type fabricLoaderVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type fabricInstallerVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// latestStable returns the first stable entry in a Fabric meta versions
// list -- these lists are already newest-first, same convention as the
// game versions list.
func fabricLatestStableLoader(ctx context.Context) (string, error) {
	loaders, err := getJSON[[]fabricLoaderVersion](ctx, fabricMetaBase+"/versions/loader")
	if err != nil {
		return "", fmt.Errorf("fetch fabric loader versions: %w", err)
	}
	for _, l := range *loaders {
		if l.Stable {
			return l.Version, nil
		}
	}
	return "", fmt.Errorf("no stable fabric loader version found")
}

func fabricLatestStableInstaller(ctx context.Context) (string, error) {
	installers, err := getJSON[[]fabricInstallerVersion](ctx, fabricMetaBase+"/versions/installer")
	if err != nil {
		return "", fmt.Errorf("fetch fabric installer versions: %w", err)
	}
	for _, i := range *installers {
		if i.Stable {
			return i.Version, nil
		}
	}
	return "", fmt.Errorf("no stable fabric installer version found")
}

func (FabricAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	loaderVersion, err := fabricLatestStableLoader(ctx)
	if err != nil {
		return "", err
	}
	installerVersion, err := fabricLatestStableInstaller(ctx)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")
	downloadURL := fmt.Sprintf("%s/versions/loader/%s/%s/%s/server/jar", fabricMetaBase, mcVersion, loaderVersion, installerVersion)
	// Fabric's bundled-server-jar endpoint doesn't publish a checksum
	// alongside the download, so there's nothing to verify against.
	if err := downloadAndVerify(ctx, downloadURL, nil, "", jarPath); err != nil {
		return "", err
	}
	return jarPath, nil
}
