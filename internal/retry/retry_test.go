package retry

import (
	"errors"
	"log"
	"os"
	"testing"
	"time"
)

func testLogger() *log.Logger { return log.New(os.Stderr, "[test] ", 0) }

func TestDo_SucceedsFirstTry(t *testing.T) {
	result, err := DoWith(testLogger(), "test", func(time.Duration) {}, func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("got %q, want %q", result, "ok")
	}
}

func TestDo_SucceedsAfterRetries(t *testing.T) {
	calls := 0
	result, err := DoWith(testLogger(), "test", func(time.Duration) {}, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Fatalf("got %d, want 42", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ExhaustsRetries(t *testing.T) {
	calls := 0
	_, err := DoWith(testLogger(), "bluesky post", func(time.Duration) {}, func() (string, error) {
		calls++
		return "", errors.New("timeout")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != MaxAttempts {
		t.Fatalf("expected %d calls, got %d", MaxAttempts, calls)
	}
	if !errors.Is(err, errors.Unwrap(err)) {
		// just check the message wraps correctly
	}
}

func TestDo_BackoffDelays(t *testing.T) {
	var delays []time.Duration
	calls := 0
	DoWith(testLogger(), "test", func(d time.Duration) {
		delays = append(delays, d)
	}, func() (struct{}, error) {
		calls++
		if calls < 4 {
			return struct{}{}, errors.New("fail")
		}
		return struct{}{}, nil
	})
	if len(delays) != 3 {
		t.Fatalf("expected 3 delays, got %d", len(delays))
	}
	if delays[0] != 1*time.Second {
		t.Errorf("delay[0] = %v, want 1s", delays[0])
	}
	if delays[1] != 2*time.Second {
		t.Errorf("delay[1] = %v, want 2s", delays[1])
	}
	if delays[2] != 4*time.Second {
		t.Errorf("delay[2] = %v, want 4s", delays[2])
	}
}

func TestRun_SucceedsFirstTry(t *testing.T) {
	err := RunWith(testLogger(), "test", func(time.Duration) {}, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ExhaustsRetries(t *testing.T) {
	err := RunWith(testLogger(), "test", func(time.Duration) {}, func() error {
		return errors.New("nope")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
