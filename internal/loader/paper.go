package loader

import "context"

// paperAPIBase points at PaperMC's v3 API (fill.papermc.io) -- their older
// v2 API (api.papermc.io) returns 410 Gone as of this writing, having been
// sunset in favor of v3.
const paperAPIBase = "https://fill.papermc.io/v3/projects/paper"

type PaperAdapter struct{}

// FetchPaperVersions lists every Minecraft version PaperMC currently
// publishes builds for, newest first.
func FetchPaperVersions(ctx context.Context) ([]string, error) {
	return fillProjectVersions(ctx, paperAPIBase)
}

func (PaperAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	return fillDownload(ctx, paperAPIBase, mcVersion, destDir)
}

// ListBuilds/DownloadBuild implement BuildLister (FR-4): Paper's fill API
// keeps every build of a version addressable, so an operator can pin a
// specific one instead of always getting the newest.
func (PaperAdapter) ListBuilds(ctx context.Context, mcVersion string) ([]BuildInfo, error) {
	return fillListBuilds(ctx, paperAPIBase, mcVersion)
}

func (PaperAdapter) DownloadBuild(ctx context.Context, mcVersion, buildID, destDir string) (string, error) {
	return fillDownloadBuild(ctx, paperAPIBase, mcVersion, buildID, destDir)
}
