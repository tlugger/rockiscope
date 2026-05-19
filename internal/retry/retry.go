package retry

import (
	"fmt"
	"log"
	"math"
	"time"
)

const MaxAttempts = 5

func Do[T any](logger *log.Logger, name string, fn func() (T, error)) (T, error) {
	return DoWith[T](logger, name, time.Sleep, fn)
}

func DoWith[T any](logger *log.Logger, name string, sleep func(time.Duration), fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < MaxAttempts {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			logger.Printf("%s: attempt %d/%d failed: %v (retrying in %s)", name, attempt, MaxAttempts, err, delay)
			sleep(delay)
		}
	}
	return zero, fmt.Errorf("%s: all %d attempts failed: %w", name, MaxAttempts, lastErr)
}

func Run(logger *log.Logger, name string, fn func() error) error {
	return RunWith(logger, name, time.Sleep, fn)
}

func RunWith(logger *log.Logger, name string, sleep func(time.Duration), fn func() error) error {
	_, err := DoWith[struct{}](logger, name, sleep, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}
