package api

import (
	"net/http"

	"craftdeck/internal/loader"
)

// handleListLoaderBuilds backs the create-instance/reinstall UI's optional
// build picker (FR-4's build-selection extension): given ?mc_version=, lists
// every build the operator can pin for that loader newest-first. Returns an
// empty array (not an error) for a recognized loader whose adapter doesn't
// implement BuildLister (e.g. vanilla, pufferfish, fabric) -- there's simply
// nothing to pick beyond "whatever Download() resolves to" -- but a 404 for
// a loader name CraftDeck doesn't know at all.
func (s *Server) handleListLoaderBuilds(w http.ResponseWriter, r *http.Request) {
	loaderName := r.PathValue("loader")
	mcVersion := r.URL.Query().Get("mc_version")
	if mcVersion == "" {
		http.Error(w, "mc_version is required", http.StatusBadRequest)
		return
	}

	adapter, ok := loader.Get(loaderName)
	if !ok {
		http.Error(w, "unknown loader", http.StatusNotFound)
		return
	}
	lister, ok := adapter.(loader.BuildLister)
	if !ok {
		writeJSON(w, http.StatusOK, []loader.BuildInfo{})
		return
	}
	builds, err := lister.ListBuilds(r.Context(), mcVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, builds)
}

// handleListVanillaVersions backs the create-instance UI's Minecraft version
// dropdown, fetched live from Mojang's version manifest (FR-1) rather than a
// hardcoded/free-text version an operator could mistype.
func (s *Server) handleListVanillaVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchVanillaVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListPaperVersions backs the create-instance UI's version dropdown
// when Paper is selected as the loader.
func (s *Server) handleListPaperVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchPaperVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListVelocityVersions backs the create-instance UI's version dropdown
// when creating a Velocity proxy instance.
func (s *Server) handleListVelocityVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchVelocityVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListPurpurVersions backs the create-instance UI's version dropdown
// when Purpur is selected as the loader.
func (s *Server) handleListPurpurVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchPurpurVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListFoliaVersions backs the create-instance UI's version dropdown
// when Folia is selected as the loader.
func (s *Server) handleListFoliaVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchFoliaVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListPufferfishVersions backs the create-instance UI's version
// dropdown when Pufferfish is selected as the loader. Unlike the other
// loaders, this list is just each Jenkins job's current latest build (see
// internal/loader/pufferfish.go), not every patch version ever released.
func (s *Server) handleListPufferfishVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchPufferfishVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListLeafVersions backs the create-instance UI's version dropdown
// when Leaf is selected as the loader.
func (s *Server) handleListLeafVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchLeafVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListFabricVersions backs the create-instance UI's version dropdown
// when Fabric is selected as the loader.
func (s *Server) handleListFabricVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchFabricVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

// handleListNeoForgeVersions backs the create-instance UI's version
// dropdown when NeoForge is selected as the loader.
func (s *Server) handleListNeoForgeVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := loader.FetchNeoForgeVersions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}
