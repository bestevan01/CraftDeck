package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"craftdeck/internal/hardware"
)

// handleGetHardware reports Active Cooler detection status plus the
// current overclock config and last benchmark result -- all one singleton
// row, so one GET covers everything the "전역 설정" tab's overclock card
// needs to decide what to render (see hardware.Config).
func (s *Server) handleGetHardware(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.hardwareSettings.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// handleRedetectCooler re-runs the one-shot Active Cooler test on demand --
// for an operator who plugs one in after the automatic startup check
// already ran and found none (see cmd/craftdeckd/main.go). Runs inline
// (a few seconds) rather than detaching, since the frontend is already
// showing a "감지 중..." state waiting on this response.
func (s *Server) handleRedetectCooler(w http.ResponseWriter, r *http.Request) {
	detected, err := hardware.DetectActiveCooler(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.hardwareSettings.SetCoolerDetected(r.Context(), detected); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg, err := s.hardwareSettings.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

type setOverclockRequest struct {
	Enabled            bool   `json:"enabled"`
	Preset             string `json:"preset"` // one of hardware.Presets' Name, or "" for custom
	ArmFreqMHz         int    `json:"arm_freq_mhz"`
	OverVoltageDeltaUV int    `json:"over_voltage_delta_uv"`
}

// handleSetOverclock writes the requested overclock into config.txt (see
// hardware.ApplyConfig) and persists it -- this does NOT reboot, since the
// firmware only reads config.txt at boot; the frontend prompts for a
// separate, explicit reboot (handleRebootForOverclock) once this succeeds.
// Rejected outright if no Active Cooler was ever confirmed present (FR
// intent: an operator without one shouldn't be able to reach this at all,
// not just have the UI hide it -- the UI gating is a courtesy, this is the
// actual enforcement).
func (s *Server) handleSetOverclock(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.hardwareSettings.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !cfg.CoolerDetected {
		http.Error(w, "overclocking requires a detected Active Cooler", http.StatusForbidden)
		return
	}

	var req setOverclockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	armFreq, overVoltageDelta := req.ArmFreqMHz, req.OverVoltageDeltaUV
	if req.Enabled && req.Preset != "" {
		preset, ok := findPreset(req.Preset)
		if !ok {
			http.Error(w, fmt.Sprintf("unknown preset %q", req.Preset), http.StatusBadRequest)
			return
		}
		armFreq, overVoltageDelta = preset.ArmFreqMHz, preset.OverVoltageDeltaUV
	}

	values := hardware.Values{Enabled: req.Enabled, Preset: req.Preset, ArmFreqMHz: armFreq, OverVoltageDeltaUV: overVoltageDelta}
	if err := hardware.ApplyConfig(values); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.hardwareSettings.SetOverclock(r.Context(), req.Enabled, req.Preset, armFreq, overVoltageDelta); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	updated, err := s.hardwareSettings.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func findPreset(name string) (hardware.Preset, bool) {
	for _, p := range hardware.Presets {
		if p.Name == name {
			return p, true
		}
	}
	return hardware.Preset{}, false
}

// handleRebootForOverclock reboots the Pi so a just-applied config.txt
// change (see handleSetOverclock) actually takes effect -- same detached-
// process shape as handleUpdateCraftdeck, since this request's own process
// is about to be killed by the reboot it's triggering.
func (s *Server) handleRebootForOverclock(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("systemd-run", "--unit=craftdeck-overclock-reboot", "--collect", "systemctl", "reboot")
	if err := cmd.Run(); err != nil {
		http.Error(w, fmt.Sprintf("failed to trigger reboot: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// benchmarkDuration is how long the stress test runs before declaring a
// clean pass -- long enough to surface a marginal overclock's instability
// (sustained load, not just a burst), short enough not to feel like the
// operator broke something while waiting.
const benchmarkDuration = 90 * time.Second

// handleStartBenchmark kicks off the CPU-load stability self-test in the
// background (see hardware.BenchmarkRunner) and returns immediately; the
// frontend polls handleBenchmarkStatus for progress/result.
func (s *Server) handleStartBenchmark(w http.ResponseWriter, r *http.Request) {
	// onDone fires ~90s after this handler already returned its 202, once
	// r.Context() has long since been cancelled (the request/connection is
	// done) -- using it here would make every SetBenchmarkResult write
	// silently fail. context.Background() is correct: this write's
	// lifetime is the benchmark run's, not this HTTP request's.
	err := s.benchmarkRunner.Start(benchmarkDuration, func(final hardware.BenchmarkStatus) {
		if err := s.hardwareSettings.SetBenchmarkResult(context.Background(), final.Result); err != nil {
			// Best-effort: the result is still visible via the in-memory
			// status until the next daemon restart even if this write fails.
			_ = err
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleBenchmarkStatus reports the in-progress or most recently finished
// benchmark run's live status (elapsed time, temperature, pass/fail).
func (s *Server) handleBenchmarkStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.benchmarkRunner.Status())
}
