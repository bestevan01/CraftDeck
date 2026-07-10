package loader

import "context"

// velocityAPIBase uses the same PaperMC "fill" v3 API family as Paper --
// Velocity is one of PaperMC's own projects, just with its own version
// numbering (Velocity's own releases, not Minecraft versions).
const velocityAPIBase = "https://fill.papermc.io/v3/projects/velocity"

type VelocityAdapter struct{}

// FetchVelocityVersions lists every Velocity version with published builds,
// newest first.
func FetchVelocityVersions(ctx context.Context) ([]string, error) {
	return fillProjectVersions(ctx, velocityAPIBase)
}

func (VelocityAdapter) Download(ctx context.Context, velocityVersion string, destDir string) (string, error) {
	return fillDownload(ctx, velocityAPIBase, velocityVersion, destDir)
}
