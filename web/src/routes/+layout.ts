// CraftDeck's web UI is served as a pure static SPA embedded in the Go
// binary (see web/embed.go, cmd/craftdeckd/main.go) — there is no Node
// server at runtime, only the prerendered fallback. Disabling SSR keeps
// every route client-rendered against craftdeckd's REST/WebSocket API.
export const ssr = false;
