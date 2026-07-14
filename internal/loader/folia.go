package loader

import "context"

// foliaAPIBase uses the same PaperMC "fill" v3 API family as Paper/Velocity
// -- Folia is PaperMC's own regionised-multithreading fork of Paper,
// published under the identical project/version/build shape (verified
// against fill.papermc.io/v3/projects/folia).
const foliaAPIBase = "https://fill.papermc.io/v3/projects/folia"

type FoliaAdapter struct{}

// FetchFoliaVersions lists every Minecraft version Folia has published
// builds for, newest first.
func FetchFoliaVersions(ctx context.Context) ([]string, error) {
	return fillProjectVersions(ctx, foliaAPIBase)
}

func (FoliaAdapter) Download(ctx context.Context, mcVersion string, destDir string) (string, error) {
	return fillDownload(ctx, foliaAPIBase, mcVersion, destDir)
}

// ListBuilds/DownloadBuild implement BuildLister (FR-4).
func (FoliaAdapter) ListBuilds(ctx context.Context, mcVersion string) ([]BuildInfo, error) {
	return fillListBuilds(ctx, foliaAPIBase, mcVersion)
}

func (FoliaAdapter) DownloadBuild(ctx context.Context, mcVersion, buildID, destDir string) (string, error) {
	return fillDownloadBuild(ctx, foliaAPIBase, mcVersion, buildID, destDir)
}
