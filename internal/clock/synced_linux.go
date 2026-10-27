package clock

import "syscall"

const (
	staUnsync = 0x0040 // STA_UNSYNC: the kernel's "clock not synchronized" flag
	timeError = 5      // TIME_ERROR: adjtimex's "clock not synchronized" state
)

// Synced reports whether NTP (systemd-timesyncd, chrony, ntpd) has
// synchronized the clock. This is the flag `timedatectl` shows as "System
// clock synchronized". Reading it needs no privileges.
var Synced = func() bool {
	var tx syscall.Timex
	state, err := syscall.Adjtimex(&tx)
	if err != nil {
		return true // can't tell; don't block the bot on it
	}
	return state != timeError && tx.Status&staUnsync == 0
}
