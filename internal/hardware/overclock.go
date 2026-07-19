package hardware

import (
	"fmt"
	"os"
	"strings"
)

// configTxtPath is where the firmware reads boot-time hardware settings on
// Raspberry Pi OS Bookworm/trixie (the OS this project targets) -- older
// releases used /boot/config.txt, but that path is a symlink into
// /boot/firmware on any image new enough to run craftdeckd's own required
// dependencies anyway.
const configTxtPath = "/boot/firmware/config.txt"

// blockStart/blockEnd delimit the region of config.txt craftdeckd owns --
// everything else in the file (whatever the operator put there themselves)
// is left untouched. Re-applying just replaces the content between these
// two lines; disabling removes the whole block.
const (
	blockStart = "# --- CraftDeck overclock (managed, do not edit) ---"
	blockEnd   = "# --- end CraftDeck overclock ---"
)

// Preset is a known-safe (per Raspberry Pi community testing, and "high"
// confirmed on the operator's own Pi 5 -- see the ApplyConfig doc comment)
// arm_freq/over_voltage_delta combination an operator can pick without
// having to know what either number means. "커스텀" in the UI bypasses this
// list entirely and lets them type their own values (see Values' bounds
// below).
//
// over_voltage_delta (microvolts) is the current Pi 5 firmware's key, not
// the legacy over_voltage integer-step key earlier Pi models used --
// confirmed on real hardware that a manually-added
// `over_voltage_delta=80000` (i.e. +0.08V) paired with `arm_freq=3000` was
// already running stably, which is why "high" below matches those exact
// numbers rather than an untested guess.
type Preset struct {
	Name               string
	Label              string
	ArmFreqMHz         int
	OverVoltageDeltaUV int
}

// Presets is deliberately conservative below "high" -- enthusiast guides
// push a well-cooled Pi 5 further, since the benchmark (see RunBenchmark)
// is the actual safety net, not these numbers. "high" itself is exactly
// what's already confirmed stable on real hardware (see Preset's doc
// comment), not a guess.
var Presets = []Preset{
	{Name: "default", Label: "기본값", ArmFreqMHz: 2400, OverVoltageDeltaUV: 0},
	{Name: "safe", Label: "안전", ArmFreqMHz: 2600, OverVoltageDeltaUV: 30000},
	{Name: "medium", Label: "보통", ArmFreqMHz: 2800, OverVoltageDeltaUV: 50000},
	{Name: "high", Label: "높음", ArmFreqMHz: 3000, OverVoltageDeltaUV: 80000},
}

// Values bounds a custom overclock request to a range that can't brick a
// boot outright (an operator can still pick an unstable combination inside
// this range -- that's what RunBenchmark is for). arm_freq below the Pi 5's
// stock 2400MHz turbo isn't really "overclocking" and is rejected too, to
// keep this feature's scope to "make it faster", not a general
// underclocking tool.
type Values struct {
	Enabled            bool
	Preset             string
	ArmFreqMHz         int
	OverVoltageDeltaUV int
}

const (
	minArmFreqMHz         = 2400
	maxArmFreqMHz         = 3200
	minOverVoltageDeltaUV = 0
	// 100000 microvolts (+0.1V) is a generous ceiling above the "high"
	// preset's confirmed-stable 80000 -- comfortably covers a custom value
	// an operator might reasonably want to try, without opening the door
	// to values community guides consider genuinely risky.
	maxOverVoltageDeltaUV = 100000
)

// fanBaseTempC0..3 are the Pi 5's stock zero-fan curve thresholds
// (millidegrees C), documented at raspberrypi.com/documentation/computers/
// config_txt.html#fan-configuration: below 50°C off, 50°C→30%, 60°C→50%,
// 67.5°C→70%, 75°C→100%, each with a 5°C hysteresis on the way back down.
const (
	fanBaseTempC0 = 50000
	fanBaseTempC1 = 60000
	fanBaseTempC2 = 67500
	fanBaseTempC3 = 75000
)

// fanOffsetSafe/Medium/High shift the whole curve down (millidegrees C) so
// heavier overclocks start cooling earlier. Sized from real sustained
// (3-minute, all-core) benchmark runs comparing stock-curve peaks against
// throttle risk (~85°C): "high" peaked at 75.1°C on the stock curve, right
// at its own top step with no margin, so it gets the full 7.5°C pull-down
// (confirmed afterward to bring the sustained peak down to 66.4°C, ~19°C of
// throttle margin -- no more is needed there). "safe" and "medium" peaked
// at 64.8°C/70.8°C on the stock curve, already 20°C/14°C below throttle
// risk with no help at all, so they only get a light nudge rather than the
// same aggressive pull-down -- more would just mean earlier, louder fan
// noise for a margin that was never actually needed.
const (
	fanOffsetSafe   = 1000
	fanOffsetMedium = 2500
	fanOffsetHigh   = 7500
)

// fanCurveOffsetUC picks how far to shift the stock fan curve down for a
// given overclock. Named presets get their fixed offset; a custom
// (non-preset) value is bucketed by how close its arm_freq is to the
// nearest named preset's, so an operator typing their own numbers still
// gets a curve roughly matched to how hard they're pushing the SoC.
func fanCurveOffsetUC(preset string, armFreqMHz int) int {
	switch preset {
	case "safe":
		return fanOffsetSafe
	case "medium":
		return fanOffsetMedium
	case "high":
		return fanOffsetHigh
	}
	switch {
	case armFreqMHz >= 2900:
		return fanOffsetHigh
	case armFreqMHz >= 2700:
		return fanOffsetMedium
	case armFreqMHz >= 2500:
		return fanOffsetSafe
	default:
		return 0
	}
}

func (v Values) Validate() error {
	if !v.Enabled {
		return nil
	}
	if v.ArmFreqMHz < minArmFreqMHz || v.ArmFreqMHz > maxArmFreqMHz {
		return fmt.Errorf("arm_freq must be between %d and %d MHz", minArmFreqMHz, maxArmFreqMHz)
	}
	if v.OverVoltageDeltaUV < minOverVoltageDeltaUV || v.OverVoltageDeltaUV > maxOverVoltageDeltaUV {
		return fmt.Errorf("over_voltage_delta must be between %d and %d microvolts", minOverVoltageDeltaUV, maxOverVoltageDeltaUV)
	}
	return nil
}

// ApplyConfig writes (or removes) craftdeckd's managed block in
// config.txt. It only takes effect after a reboot -- the firmware reads
// these keys once at boot, there's no live-apply for arm_freq/
// over_voltage_delta -- so this never triggers one itself; callers trigger
// a reboot as an explicit, separate, user-confirmed action.
func ApplyConfig(v Values) error {
	if err := v.Validate(); err != nil {
		return err
	}

	data, err := os.ReadFile(configTxtPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", configTxtPath, err)
	}
	lines := strings.Split(string(data), "\n")

	startIdx, endIdx := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case blockStart:
			startIdx = i
		case blockEnd:
			endIdx = i
		}
	}

	var block []string
	if v.Enabled {
		block = []string{
			blockStart,
			fmt.Sprintf("arm_freq=%d", v.ArmFreqMHz),
			fmt.Sprintf("over_voltage_delta=%d", v.OverVoltageDeltaUV),
		}
		if offset := fanCurveOffsetUC(v.Preset, v.ArmFreqMHz); offset > 0 {
			block = append(block,
				fmt.Sprintf("dtparam=fan_temp0=%d", fanBaseTempC0-offset),
				fmt.Sprintf("dtparam=fan_temp1=%d", fanBaseTempC1-offset),
				fmt.Sprintf("dtparam=fan_temp2=%d", fanBaseTempC2-offset),
				fmt.Sprintf("dtparam=fan_temp3=%d", fanBaseTempC3-offset),
			)
		}
		block = append(block, blockEnd)
	}

	var result []string
	if startIdx >= 0 && endIdx > startIdx {
		result = append(result, lines[:startIdx]...)
		result = append(result, block...)
		result = append(result, lines[endIdx+1:]...)
	} else {
		result = append(result, lines...)
		result = append(result, block...)
	}

	return os.WriteFile(configTxtPath, []byte(strings.Join(result, "\n")), 0o644)
}
