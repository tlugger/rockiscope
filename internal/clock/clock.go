// Package clock decides when the system clock can be trusted. A Raspberry Pi
// has no hardware clock: after a power cut it boots with whatever time
// systemd last saved, which can be hours behind, and NTP corrects it a minute
// or so later. A bot that plans its day before then sleeps through it.
package clock

import (
	"log"
	"time"
)

// WaitForSync blocks until the kernel reports the clock as NTP-synchronized,
// or maxWait passes (a box with no time daemon would otherwise never start).
// It reports whether the clock was synchronized.
func WaitForSync(logger *log.Logger, sleep func(time.Duration), poll, maxWait time.Duration) bool {
	if Synced() {
		return true
	}
	logger.Printf("system clock isn't NTP-synchronized yet (reads %s); waiting before planning anything", time.Now().Format(time.RFC3339))
	for waited := time.Duration(0); waited < maxWait; waited += poll {
		sleep(poll)
		if Synced() {
			logger.Printf("system clock synchronized: %s", time.Now().Format(time.RFC3339))
			return true
		}
	}
	logger.Printf("WARNING: system clock still not synchronized after %s; continuing with %s", maxWait, time.Now().Format(time.RFC3339))
	return false
}
