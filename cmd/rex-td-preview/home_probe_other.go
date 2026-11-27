//go:build !darwin && !linux

package main

import (
	"os"
	"time"
)

func inputReady(_ *os.File, _ time.Time) bool { return false }
func probeHomeTerminal(_, _ *os.File) homeTerminalProbe {
	return homeTerminalProbe{}
}
