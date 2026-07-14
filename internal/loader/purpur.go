package loader

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
)

// purpurAPIBase is PurpurMC's own distribution API (not part of PaperMC's
// "fill" family, despite Purpur being a Paper fork) -- verified against
// api.purpurmc.org/v2, which lists versions oldest-first and exposes an
// MD5 checksum per build rather than PaperMC's SHA-256.
const purpurAPIBase = "https://api.purpurmc.org/v2/purpur"

type PurpurAdapter struct{}

type purpurProject struct {
	Versions []string `json:"versions"`
}

// FetchPurpurVersions lists every Minecraft version Purpur has published
// builds for, newest first (the API itself returns them oldest-first).
func FetchPurpurVersions(ctx context.Context) ([]string, error) {
	project, err := getJSON[purpurProject](ctx, purpurAPIBase)
	if err != nil {
		return nil, fmt.Errorf("fetch purpur versions: %w", err)
	}
	versions := make([]string, len(project.Versions))
	for i, v := range project.Versions {
		versions[len(project.Versions)-1-i] = v
	}
	return versions, nil
}

type purpurLatestBuild struct {
	Build string `json:"build"`
	MD5   string `json:"md5"`
}

func (PurpurAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	build, err := getJSON[purpurLatestBuild](ctx, fmt.Sprintf("%s/%s/latest", purpurAPIBase, mcVersion))
	if err != nil {
		return "", fmt.Errorf("fetch latest purpur build for %q: %w", mcVersion, err)
	}
	if build.Build == "" {
		return "", fmt.Errorf("no purpur build found for version %q", mcVersion)
	}
	return purpurDownloadBuild(ctx, mcVersion, build.Build, build.MD5, destDir)
}

func purpurDownloadBuild(ctx context.Context, mcVersion, buildID, md5Hex, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")
	downloadURL := fmt.Sprintf("%s/%s/%s/download", purpurAPIBase, mcVersion, buildID)
	if err := downloadAndVerify(ctx, downloadURL, md5.New, md5Hex, jarPath); err != nil {
		return "", err
	}
	return jarPath, nil
}

type purpurVersionBuilds struct {
	Builds struct {
		All []string `json:"all"`
	} `json:"builds"`
}

// ListBuilds/DownloadBuild implement BuildLister (FR-4): the "all" list is
// oldest-first per Purpur's own API, so it's reversed here to match every
// other adapter's newest-first convention. Purpur doesn't expose a
// per-build timestamp in this listing (only in each build's own detail
// endpoint, which would mean one extra request per build just to display a
// date), so BuildInfo.Time is left empty here.
func (PurpurAdapter) ListBuilds(ctx context.Context, mcVersion string) ([]BuildInfo, error) {
	resp, err := getJSON[purpurVersionBuilds](ctx, fmt.Sprintf("%s/%s", purpurAPIBase, mcVersion))
	if err != nil {
		return nil, fmt.Errorf("fetch purpur builds for %q: %w", mcVersion, err)
	}
	out := make([]BuildInfo, len(resp.Builds.All))
	for i, b := range resp.Builds.All {
		out[len(resp.Builds.All)-1-i] = BuildInfo{ID: b}
	}
	return out, nil
}

func (PurpurAdapter) DownloadBuild(ctx context.Context, mcVersion, buildID, destDir string) (string, error) {
	build, err := getJSON[purpurLatestBuild](ctx, fmt.Sprintf("%s/%s/%s", purpurAPIBase, mcVersion, buildID))
	if err != nil {
		return "", fmt.Errorf("fetch purpur build %s for %q: %w", buildID, mcVersion, err)
	}
	return purpurDownloadBuild(ctx, mcVersion, buildID, build.MD5, destDir)
}
