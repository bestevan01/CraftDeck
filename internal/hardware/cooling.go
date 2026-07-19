package hardware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DetectActiveCooler reports whether a Raspberry Pi 5 Active Cooler is
// physically attached. The obvious approach -- checking whether
// /sys/class/thermal/cooling_device*/type is "pwm-fan" -- doesn't work:
// that node is declared in the Pi 5's device tree unconditionally, so the
// kernel driver probes it whether or not anything is actually plugged
// into the fan header (confirmed on real hardware). And the cooler's own
// zero-fan feature means it stays stopped below 50°C anyway, so even
// "is it currently spinning" tells you nothing at idle temperatures.
//
// What actually proves a fan is there: force the PWM duty cycle up via
// the cooling_device's cur_state and watch whether the tachometer
// (hwmon's fan1_input) responds with real RPM. Confirmed on real
// hardware: cur_state=max_state produced 5253 RPM within a few seconds,
// dropping back to 0 once cur_state was restored. No fan wired to the
// header would just stay at 0 RPM regardless of duty cycle.
//
// The forced cur_state is always restored to 0 before returning (even on
// error), and the kernel's own thermal governor overwrites cur_state
// again on its next polling cycle regardless, so this is safe to run on
// a live system -- but it's still an active, physically-visible test
// (spins the fan up), which is why callers only run it once (see
// cmd/craftdeckd/main.go) rather than on every restart.
func DetectActiveCooler(ctx context.Context) (bool, error) {
	model, err := os.ReadFile("/proc/device-tree/model")
	if err != nil || !strings.Contains(string(model), "Raspberry Pi 5") {
		return false, nil // not a Pi 5 (or couldn't tell) -- nothing to test
	}

	coolingDevice, err := findCoolingDevice("pwm-fan")
	if err != nil {
		return false, nil // no pwm-fan node at all -- can't be a cooler
	}
	fanInputPath, err := findHwmonAttr("pwmfan", "fan1_input")
	if err != nil {
		return false, nil
	}

	maxStateRaw, err := os.ReadFile(filepath.Join(coolingDevice, "max_state"))
	if err != nil {
		return false, fmt.Errorf("read cooling device max_state: %w", err)
	}
	maxState := strings.TrimSpace(string(maxStateRaw))
	curStatePath := filepath.Join(coolingDevice, "cur_state")

	defer func() { _ = os.WriteFile(curStatePath, []byte("0"), 0o644) }()

	if err := os.WriteFile(curStatePath, []byte(maxState), 0o644); err != nil {
		return false, fmt.Errorf("force cooling device to max_state: %w", err)
	}

	const pollInterval = time.Second
	const maxPolls = 5
	for i := 0; i < maxPolls; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(pollInterval):
		}
		rpm, err := readFanRPM(fanInputPath)
		if err == nil && rpm > 0 {
			return true, nil // deferred write above still restores cur_state=0
		}
	}
	return false, nil
}

func readFanRPM(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(raw)))
}

// findCoolingDevice returns the /sys/class/thermal/cooling_deviceN
// directory whose "type" attribute equals typ.
func findCoolingDevice(typ string) (string, error) {
	matches, err := filepath.Glob("/sys/class/thermal/cooling_device*")
	if err != nil {
		return "", err
	}
	for _, m := range matches {
		data, err := os.ReadFile(filepath.Join(m, "type"))
		if err == nil && strings.TrimSpace(string(data)) == typ {
			return m, nil
		}
	}
	return "", fmt.Errorf("no cooling device of type %q found", typ)
}

// findHwmonAttr returns the full path to attr under the
// /sys/class/hwmon/hwmonN directory whose "name" attribute equals name.
func findHwmonAttr(name, attr string) (string, error) {
	matches, err := filepath.Glob("/sys/class/hwmon/hwmon*")
	if err != nil {
		return "", err
	}
	for _, m := range matches {
		data, err := os.ReadFile(filepath.Join(m, "name"))
		if err == nil && strings.TrimSpace(string(data)) == name {
			return filepath.Join(m, attr), nil
		}
	}
	return "", fmt.Errorf("no hwmon device named %q found", name)
}
