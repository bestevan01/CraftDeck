// Package web embeds the SvelteKit static build (adapter-static output,
// see svelte.config.js) into the craftdeckd binary per requirements.md
// FR-41 ("apt install" should need nothing beyond the package itself).
//
// The build/ directory checked in alongside this file is a placeholder;
// run `npm run build` in this directory before `go build` to embed the
// real frontend.
package web

import "embed"

//go:embed all:build
var Assets embed.FS
