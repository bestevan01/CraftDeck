// Package loader downloads Minecraft server jars from each loader's
// official distribution point (requirements.md FR-1, FR-2). Only the
// Vanilla adapter is implemented so far; Paper/Fabric/Forge/Velocity/
// BungeeCord adapters follow the same Adapter interface.
package loader

import "context"

// Adapter fetches one loader's server jar for a given Minecraft version and
// writes it to destDir/server.jar.
type Adapter interface {
	// Download places the server jar at destDir/server.jar, returning its
	// path. mcVersion is the Minecraft version string (e.g. "1.21").
	Download(ctx context.Context, mcVersion string, destDir string) (jarPath string, err error)
}

var registry = map[string]Adapter{
	"vanilla": VanillaAdapter{},
}

func Get(loaderName string) (Adapter, bool) {
	a, ok := registry[loaderName]
	return a, ok
}
