// Package modrinth is a minimal client for the Modrinth API (requirements.md
// FR-5, FR-6): searching for plugins/mods, listing a project's versions, and
// resolving required dependencies -- just what installing a plugin needs,
// not a general-purpose API wrapper.
package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const apiBase = "https://api.modrinth.com/v2"

// SearchHit is the subset of a search result CraftDeck's UI shows.
type SearchHit struct {
	ProjectID   string `json:"project_id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Downloads   int    `json:"downloads"`
	IconURL     string `json:"icon_url"`
}

type searchResponse struct {
	Hits []SearchHit `json:"hits"`
}

// Search looks up plugins (or mods) matching query, filtered to projects
// compatible with the given loader and Minecraft version (FR-6a).
// projectType is "plugin" or "mod".
func Search(ctx context.Context, query, projectType, loader, mcVersion string) ([]SearchHit, error) {
	facets := fmt.Sprintf(
		`[["project_type:%s"],["categories:%s"],["versions:%s"]]`,
		projectType, loader, mcVersion,
	)
	params := url.Values{
		"query":  {query},
		"facets": {facets},
		"limit":  {"20"},
	}
	var resp searchResponse
	if err := getJSON(ctx, apiBase+"/search?"+params.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("modrinth search: %w", err)
	}
	return resp.Hits, nil
}

// VersionFile is one downloadable file attached to a project version --
// almost always exactly one (the plugin/mod jar itself).
type VersionFile struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Hashes   map[string]string `json:"hashes"` // "sha1", "sha512"
}

// Dependency is one entry in a version's dependency list.
type Dependency struct {
	VersionID      string `json:"version_id"`
	ProjectID      string `json:"project_id"`
	DependencyType string `json:"dependency_type"` // "required", "optional", "incompatible", "embedded"
}

// Version is one published version of a project.
type Version struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"project_id"`
	Name          string        `json:"name"`
	VersionNumber string        `json:"version_number"`
	VersionType   string        `json:"version_type"` // "release", "beta", or "alpha"
	Changelog     string        `json:"changelog"`
	DatePublished string        `json:"date_published"`
	GameVersions  []string      `json:"game_versions"`
	Loaders       []string      `json:"loaders"`
	Dependencies  []Dependency  `json:"dependencies"`
	Files         []VersionFile `json:"files"`
}

// Project is the subset of a Modrinth project's details CraftDeck's UI
// shows -- enough for the detail modal (description, categories) plus the
// display name already used for installed plugins/mods.
type Project struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	IconURL     string   `json:"icon_url"`
}

// GetProject fetches projectID's details (FR-6, display name for installed
// plugins/mods, plus description/categories for the detail modal).
func GetProject(ctx context.Context, projectID string) (*Project, error) {
	var p Project
	if err := getJSON(ctx, apiBase+"/project/"+url.PathEscape(projectID), &p); err != nil {
		return nil, fmt.Errorf("modrinth project: %w", err)
	}
	return &p, nil
}

// ProjectVersions lists every published version of a project, newest first
// (Modrinth's own ordering).
func ProjectVersions(ctx context.Context, projectID string) ([]Version, error) {
	var versions []Version
	if err := getJSON(ctx, apiBase+"/project/"+url.PathEscape(projectID)+"/version", &versions); err != nil {
		return nil, fmt.Errorf("modrinth project versions: %w", err)
	}
	return versions, nil
}

// GetVersion fetches a single version by ID directly -- used when an
// operator picks a specific version/channel from the detail modal instead
// of letting BestVersion auto-select one.
func GetVersion(ctx context.Context, versionID string) (*Version, error) {
	var v Version
	if err := getJSON(ctx, apiBase+"/version/"+url.PathEscape(versionID), &v); err != nil {
		return nil, fmt.Errorf("modrinth version: %w", err)
	}
	return &v, nil
}

// versionTypeRank orders Modrinth's version_type so a release always wins
// over a beta, which always wins over an alpha -- BestVersion below prefers
// the lowest rank it can find. An empty/unrecognized value (shouldn't
// happen against the real API, but cheap to handle) sorts last rather than
// being treated as equal to release.
func versionTypeRank(t string) int {
	switch t {
	case "release":
		return 0
	case "beta":
		return 1
	case "alpha":
		return 2
	default:
		return 3
	}
}

// BestVersion picks the newest *release* version of projectID compatible
// with loader and mcVersion (FR-6a), falling back to the newest beta, and
// then the newest alpha, only if no more-stable tier has a compatible
// version at all -- picking whatever compatible version merely has the
// most recent date_published (Modrinth's own listing order, see
// ProjectVersions) used to mean a beta uploaded after the last release
// would win just for being newer, even with a perfectly good release
// available (confirmed: exactly this, installing a beta over an existing
// release). versions is newest-first, so within a tier the first match
// found is already that tier's newest; hitting a release immediately stops
// the search since nothing later in the list could outrank it.
func BestVersion(ctx context.Context, projectID, loader, mcVersion string) (*Version, error) {
	versions, err := ProjectVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var best *Version
	for i := range versions {
		v := &versions[i]
		if !v.SupportsLoaderAndMC(loader, mcVersion) {
			continue
		}
		if best == nil || versionTypeRank(v.VersionType) < versionTypeRank(best.VersionType) {
			best = v
		}
		if v.VersionType == "release" {
			break
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no version of project %s supports loader %q and Minecraft %q", projectID, loader, mcVersion)
	}
	return best, nil
}

// SupportsLoaderAndMC reports whether v can actually run under loader and
// mcVersion -- the same compatibility check BestVersion uses, exported so
// callers that need to list every compatible version themselves (e.g. the
// detail-modal endpoint, which shows the operator every matching
// Release/Beta/Alpha instead of just the one BestVersion would pick) don't
// have to duplicate it.
func (v *Version) SupportsLoaderAndMC(loader, mcVersion string) bool {
	return containsFold(v.Loaders, loader) && contains(v.GameVersions, mcVersion)
}

// PrimaryFile returns a version's primary downloadable file (falling back
// to the first file if none is explicitly marked primary).
func (v *Version) PrimaryFile() (VersionFile, error) {
	for _, f := range v.Files {
		if f.Primary {
			return f, nil
		}
	}
	if len(v.Files) > 0 {
		return v.Files[0], nil
	}
	return VersionFile{}, fmt.Errorf("version %s has no files", v.ID)
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "craftdeck/0.1 (self-hosted Minecraft server manager)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, u)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
