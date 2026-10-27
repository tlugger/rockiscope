//go:build !linux

package clock

// Synced is always true off Linux (development machines keep good time).
var Synced = func() bool { return true }
