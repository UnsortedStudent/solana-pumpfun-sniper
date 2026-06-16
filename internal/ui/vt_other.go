//go:build !windows

package ui

// enableVT is a no-op on non-Windows platforms, where ANSI escape codes work natively.
func enableVT() {}
