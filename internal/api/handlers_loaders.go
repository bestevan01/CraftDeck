package api

import (
	"net/http"

	"craftdeck/internal/loader"
)

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
