package loader

import "context"

// velocityAPIBase uses the same PaperMC "fill" v3 API family as Paper --
// Velocity is one of PaperMC's own projects, just with its own version
// numbering (Velocity's own releases, not Minecraft versions).
const velocityAPIBase = "https://fill.papermc.io/v3/projects/velocity"

type VelocityAdapter struct{}

// FetchVelocityVersions lists every Velocity version the fill API knows
// about, newest first -- note this can include a version with no published
// build yet (see FetchLatestBuildableVelocityVersion), so don't assume
// index 0 is downloadable.
func FetchVelocityVersions(ctx context.Context) ([]string, error) {
	return fillProjectVersions(ctx, velocityAPIBase)
}

// FetchLatestBuildableVelocityVersion returns the newest Velocity version
// that actually has a downloadable build. Callers that need "the version to
// install" (ensureProxyInstance, handleUpgradeProxy) should use this instead
// of FetchVelocityVersions()[0].
func FetchLatestBuildableVelocityVersion(ctx context.Context) (string, error) {
	return fillLatestBuildableVersion(ctx, velocityAPIBase)
}

func (VelocityAdapter) Download(ctx context.Context, velocityVersion string, destDir string) (string, error) {
	return fillDownload(ctx, velocityAPIBase, velocityVersion, destDir)
}
