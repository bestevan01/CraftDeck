// Package version holds craftdeckd's own build version, checked against
// the apt repository to tell the operator when an upgrade is available
// (see internal/api/handlers_system.go's handleCraftdeckVersion).
package version

// Version is overridden at build time via
// `-ldflags "-X craftdeck/internal/version.Version=x.y.z"` (see
// .github/workflows/release.yml). Left at "dev" for local builds that
// don't pass that flag, which never claims an update is available since
// nothing on the apt repo will ever equal "dev".
var Version = "dev"
