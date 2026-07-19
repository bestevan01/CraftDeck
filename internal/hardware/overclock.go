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

// Preset is a known-safe (per Raspberry Pi community testing) arm_freq/
// over_voltage combination an operator can pick without having to know
// what either number means. "커스텀" in the UI bypasses this list entirely
// and lets them type their own values (see Values' bounds below).
type Preset struct {
	Name        string
	Label       string
	ArmFreqMHz  int
	OverVoltage int
}

// Presets is deliberately conservative -- "높음" is well short of what
// enthusiast guides push a well-cooled Pi 5 to, since the benchmark (see
// RunBenchmark) is the actual safety net, not these numbers.
var Presets = []Preset{
	{Name: "default", Label: "기본값", ArmFreqMHz: 2400, OverVoltage: 0},
	{Name: "safe", Label: "안전", ArmFreqMHz: 2600, OverVoltage: 3},
	{Name: "medium", Label: "보통", ArmFreqMHz: 2800, OverVoltage: 6},
	{Name: "high", Label: "높음", ArmFreqMHz: 3000, OverVoltage: 8},
}

// Values bounds a custom overclock request to a range that can't brick a
// boot outright (an operator can still pick an unstable combination inside
// this range -- that's what RunBenchmark is for). arm_freq below the Pi 5's
// stock 2400MHz turbo isn't really "overclocking" and is rejected too, to
// keep this feature's scope to "make it faster", not a general
// underclocking tool.
type Values struct {
	Enabled     bool
	Preset      string
	ArmFreqMHz  int
	OverVoltage int
}

const (
	minArmFreqMHz = 2400
	maxArmFreqMHz = 3200
	minOverVoltage = 0
	maxOverVoltage = 10
)

func (v Values) Validate() error {
	if !v.Enabled {
		return nil
	}
	if v.ArmFreqMHz < minArmFreqMHz || v.ArmFreqMHz > maxArmFreqMHz {
		return fmt.Errorf("arm_freq must be between %d and %d MHz", minArmFreqMHz, maxArmFreqMHz)
	}
	if v.OverVoltage < minOverVoltage || v.OverVoltage > maxOverVoltage {
		return fmt.Errorf("over_voltage must be between %d and %d", minOverVoltage, maxOverVoltage)
	}
	return nil
}

// ApplyConfig writes (or removes) craftdeckd's managed block in
// config.txt. It only takes effect after a reboot -- the firmware reads
// these keys once at boot, there's no live-apply for arm_freq/
// over_voltage -- so this never triggers one itself; callers trigger a
// reboot as an explicit, separate, user-confirmed action.
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
			fmt.Sprintf("over_voltage=%d", v.OverVoltage),
			blockEnd,
		}
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
