package hardware

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BenchmarkStatus is polled by the frontend (GET /api/system/overclock/
// benchmark/status) every second while a stability self-test is running --
// CurrentTempC is the live reading for that display, while
// Min/Max/AvgTempC accumulate from every sample taken during the run so
// the frontend can show a summary once it ends. Both stay populated
// (holding the finished run's numbers) until the next run starts.
type BenchmarkStatus struct {
	Running      bool    `json:"running"`
	ElapsedSec   int     `json:"elapsed_sec"`
	TotalSec     int     `json:"total_sec"`
	CurrentTempC float64 `json:"current_temp_c"`
	MinTempC     float64 `json:"min_temp_c"`
	MaxTempC     float64 `json:"max_temp_c"`
	AvgTempC     float64 `json:"avg_temp_c"`
	Result       string  `json:"result"` // "", "pass", "fail"
	UnderVoltage bool    `json:"under_voltage_detected"`
	Throttled    bool    `json:"throttled_detected"`
}

// BenchmarkRunner drives a single in-process stability test at a time --
// pure-Go CPU load (no stress-ng/extra apt dependency, matching NFR-9)
// across every core, cross-checked against vcgencmd's own under-voltage/
// throttling flags rather than just "did it crash", since a Pi under a bad
// overclock usually throttles or reboots silently rather than erroring out
// where Go code could catch it.
type BenchmarkRunner struct {
	mu     sync.Mutex
	status BenchmarkStatus
}

func NewBenchmarkRunner() *BenchmarkRunner {
	return &BenchmarkRunner{}
}

func (r *BenchmarkRunner) Status() BenchmarkStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Start kicks off a benchmark in the background and returns immediately --
// same "can't block the HTTP response on something long-running" shape as
// handleUpdateCraftdeck. onDone is called with the final status once the
// run completes, so the caller can persist the result (see
// Repository.SetBenchmarkResult) without BenchmarkRunner itself needing a
// DB handle.
func (r *BenchmarkRunner) Start(duration time.Duration, onDone func(BenchmarkStatus)) error {
	r.mu.Lock()
	if r.status.Running {
		r.mu.Unlock()
		return fmt.Errorf("a benchmark is already running")
	}
	r.status = BenchmarkStatus{Running: true, TotalSec: int(duration.Seconds())}
	r.mu.Unlock()

	go r.run(duration, onDone)
	return nil
}

func (r *BenchmarkRunner) run(duration time.Duration, onDone func(BenchmarkStatus)) {
	ctx, cancel := context.WithTimeout(context.Background(), duration+5*time.Second)
	defer cancel()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			busyLoop(stop)
		}()
	}

	// Baseline "has occurred" bits before the load starts, so a genuinely
	// pre-existing under-voltage condition (e.g. a marginal power supply,
	// unrelated to this specific overclock) doesn't get misattributed to
	// this run -- only *new* occurrences during the test count.
	baselineOccurred, _ := readThrottledOccurred(ctx)

	start := time.Now()
	deadline := start.Add(duration)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var tempSum float64
	var tempCount int

	failed := false
	for time.Now().Before(deadline) && !failed {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			failed = true
		}
		current, _ := readThrottledCurrent(ctx)
		temp, tempErr := measureTempC(ctx)

		r.mu.Lock()
		r.status.ElapsedSec = int(time.Since(start).Seconds())
		if tempErr == nil {
			r.status.CurrentTempC = temp
			if tempCount == 0 || temp < r.status.MinTempC {
				r.status.MinTempC = temp
			}
			if tempCount == 0 || temp > r.status.MaxTempC {
				r.status.MaxTempC = temp
			}
			tempSum += temp
			tempCount++
			r.status.AvgTempC = tempSum / float64(tempCount)
		}
		if current.underVoltage {
			r.status.UnderVoltage = true
		}
		if current.throttled || current.freqCapped {
			r.status.Throttled = true
		}
		if r.status.UnderVoltage || r.status.Throttled {
			failed = true
		}
		r.mu.Unlock()
	}

	close(stop)
	wg.Wait()

	finalOccurred, _ := readThrottledOccurred(ctx)

	r.mu.Lock()
	if finalOccurred.underVoltage && !baselineOccurred.underVoltage {
		r.status.UnderVoltage = true
	}
	if (finalOccurred.throttled && !baselineOccurred.throttled) || (finalOccurred.freqCapped && !baselineOccurred.freqCapped) {
		r.status.Throttled = true
	}
	if r.status.UnderVoltage || r.status.Throttled {
		r.status.Result = "fail"
	} else {
		r.status.Result = "pass"
	}
	r.status.Running = false
	final := r.status
	r.mu.Unlock()

	if onDone != nil {
		onDone(final)
	}
}

// busyLoop pegs one core at ~100% with floating-point work until stop is
// closed -- no external dependency, just enough sustained load to surface
// a marginal overclock's instability within the test window.
func busyLoop(stop <-chan struct{}) {
	x := 0.0001
	for {
		select {
		case <-stop:
			return
		default:
			for i := 0; i < 1_000_000; i++ {
				x = x*1.0000001 + 0.0000001
				if x > 1e6 {
					x = 0.0001
				}
			}
		}
	}
}

type throttledBits struct {
	underVoltage bool
	freqCapped   bool
	throttled    bool
}

// readThrottledCurrent parses vcgencmd's real-time bits (0-3): whether
// under-voltage/frequency-capping/throttling is happening *right now*.
func readThrottledCurrent(ctx context.Context) (throttledBits, error) {
	bits, err := readThrottledRaw(ctx)
	if err != nil {
		return throttledBits{}, err
	}
	return throttledBits{
		underVoltage: bits&(1<<0) != 0,
		freqCapped:   bits&(1<<1) != 0,
		throttled:    bits&(1<<2) != 0,
	}, nil
}

// readThrottledOccurred parses vcgencmd's "has happened since boot" bits
// (16-18) -- a backstop for events that flip on and off between our 1s
// polls of the current-state bits.
func readThrottledOccurred(ctx context.Context) (throttledBits, error) {
	bits, err := readThrottledRaw(ctx)
	if err != nil {
		return throttledBits{}, err
	}
	return throttledBits{
		underVoltage: bits&(1<<16) != 0,
		freqCapped:   bits&(1<<17) != 0,
		throttled:    bits&(1<<18) != 0,
	}, nil
}

// readThrottledRaw runs `vcgencmd get_throttled`, which prints a single
// line like "throttled=0x50005" -- a bitmask documented at
// https://www.raspberrypi.com/documentation/computers/os.html#get_throttled.
func readThrottledRaw(ctx context.Context) (uint32, error) {
	out, err := exec.CommandContext(ctx, "vcgencmd", "get_throttled").Output()
	if err != nil {
		return 0, fmt.Errorf("vcgencmd get_throttled: %w", err)
	}
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "throttled=")
	s = strings.TrimPrefix(s, "0x")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse vcgencmd get_throttled output %q: %w", out, err)
	}
	return uint32(v), nil
}

// measureTempC runs `vcgencmd measure_temp`, which prints a line like
// "temp=52.3'C".
func measureTempC(ctx context.Context) (float64, error) {
	out, err := exec.CommandContext(ctx, "vcgencmd", "measure_temp").Output()
	if err != nil {
		return 0, fmt.Errorf("vcgencmd measure_temp: %w", err)
	}
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "temp=")
	s = strings.TrimSuffix(s, "'C")
	return strconv.ParseFloat(s, 64)
}
