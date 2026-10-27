package clock

import (
	"io"
	"log"
	"testing"
	"time"
)

func TestWaitForSync(t *testing.T) {
	orig := Synced
	t.Cleanup(func() { Synced = orig })
	logger := log.New(io.Discard, "", 0)

	calls := 0
	Synced = func() bool { calls++; return calls >= 3 }
	var slept time.Duration
	if !WaitForSync(logger, func(d time.Duration) { slept += d }, time.Second, time.Minute) {
		t.Error("should report synchronized")
	}
	if slept != 2*time.Second {
		t.Errorf("slept %s, want 2s", slept)
	}

	Synced = func() bool { return false }
	slept = 0
	if WaitForSync(logger, func(d time.Duration) { slept += d }, 10*time.Second, time.Minute) {
		t.Error("should give up unsynchronized")
	}
	if slept != time.Minute {
		t.Errorf("slept %s, want the full minute", slept)
	}
}

// On this machine the real check must at least run without blocking.
func TestSynced_Runs(t *testing.T) { _ = Synced() }
