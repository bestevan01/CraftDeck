package loader

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// leafAPIBase is LeafMC's own distribution API -- structured like PaperMC's
// now-retired v2 API (project -> versions -> builds -> downloads), self-
// hosted at api.leafmc.one rather than under fill.papermc.io. "Leaf"
// (Winds-Studio/Leaf) is a distinct, more actively maintained project from
// a similarly-named "Leaves" (LeavesMC/Leaves) -- verified via GitHub: Leaf
// has more stars/forks and was pushed to within the last day, while Leaves
// hadn't been touched in two months. CraftDeck supports Leaf, not Leaves.
const leafAPIBase = "https://api.leafmc.one/v2/projects/leaf"

type LeafAdapter struct{}

type leafProject struct {
	Versions []string `json:"versions"`
}

// FetchLeafVersions lists every Minecraft version Leaf has published builds
// for, newest first (the API's own ordering isn't reliably sorted, so this
// re-sorts numerically via compareVersions -- see pufferfish.go).
func FetchLeafVersions(ctx context.Context) ([]string, error) {
	project, err := getJSON[leafProject](ctx, leafAPIBase)
	if err != nil {
		return nil, fmt.Errorf("fetch leaf versions: %w", err)
	}
	versions := append([]string(nil), project.Versions...)
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	return versions, nil
}

type leafBuild struct {
	Build     int    `json:"build"`
	Channel   string `json:"channel"` // "default" (stable) or "experimental"
	Time      string `json:"time"`
	Downloads struct {
		Primary struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		} `json:"primary"`
	} `json:"downloads"`
}

type leafBuildsResponse struct {
	Builds []leafBuild `json:"builds"`
}

func leafFetchBuilds(ctx context.Context, mcVersion string) ([]leafBuild, error) {
	resp, err := getJSON[leafBuildsResponse](ctx, fmt.Sprintf("%s/versions/%s/builds", leafAPIBase, mcVersion))
	if err != nil {
		return nil, fmt.Errorf("fetch builds for %q: %w", mcVersion, err)
	}
	return resp.Builds, nil
}

func leafDownloadBuildJar(ctx context.Context, mcVersion string, build leafBuild, destDir string) (string, error) {
	if build.Downloads.Primary.Name == "" {
		return "", fmt.Errorf("build %d for %q has no download", build.Build, mcVersion)
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	jarPath := filepath.Join(destDir, "server.jar")
	downloadURL := fmt.Sprintf("%s/versions/%s/builds/%d/downloads/%s", leafAPIBase, mcVersion, build.Build, build.Downloads.Primary.Name)
	if err := downloadAndVerify(ctx, downloadURL, sha256.New, build.Downloads.Primary.SHA256, jarPath); err != nil {
		return "", err
	}
	return jarPath, nil
}

func (LeafAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	builds, err := leafFetchBuilds(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	if len(builds) == 0 {
		return "", fmt.Errorf("no builds found for version %q", mcVersion)
	}

	// Builds are listed oldest-first (ascending build number); walk from
	// the end to prefer the newest "default" (stable) build, falling back
	// to the newest build of any channel if none is marked default yet.
	build := builds[len(builds)-1]
	for i := len(builds) - 1; i >= 0; i-- {
		if builds[i].Channel == "default" {
			build = builds[i]
			break
		}
	}
	return leafDownloadBuildJar(ctx, mcVersion, build, destDir)
}

// ListBuilds/DownloadBuild implement BuildLister (FR-4).
func (LeafAdapter) ListBuilds(ctx context.Context, mcVersion string) ([]BuildInfo, error) {
	builds, err := leafFetchBuilds(ctx, mcVersion)
	if err != nil {
		return nil, err
	}
	out := make([]BuildInfo, len(builds))
	for i, b := range builds {
		// Oldest-first from the API; reverse to match every other
		// adapter's newest-first convention.
		out[len(builds)-1-i] = BuildInfo{ID: strconv.Itoa(b.Build), Channel: b.Channel, Time: b.Time}
	}
	return out, nil
}

func (LeafAdapter) DownloadBuild(ctx context.Context, mcVersion, buildID, destDir string) (string, error) {
	builds, err := leafFetchBuilds(ctx, mcVersion)
	if err != nil {
		return "", err
	}
	for _, b := range builds {
		if strconv.Itoa(b.Build) == buildID {
			return leafDownloadBuildJar(ctx, mcVersion, b, destDir)
		}
	}
	return "", fmt.Errorf("build %q not found for version %q", buildID, mcVersion)
}
