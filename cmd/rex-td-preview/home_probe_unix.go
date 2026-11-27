//go:build darwin || linux

package main

import (
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func inputReady(file *os.File, until time.Time) bool {
	poll := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
	for time.Now().Before(until) {
		ms := max(1, int(time.Until(until)/time.Millisecond))
		ready, err := unix.Poll(poll, ms)
		if err == unix.EINTR {
			continue
		}
		return err == nil && ready > 0 && poll[0].Revents&unix.POLLIN != 0
	}
	return false
}

func probeHomeTerminal(input, output *os.File) homeTerminalProbe {
	var result homeTerminalProbe
	if !term.IsTerminal(input.Fd()) || !term.IsTerminal(output.Fd()) {
		return result
	}
	mode, err := term.MakeRaw(input.Fd())
	if err != nil {
		return result
	}
	defer term.Restore(input.Fd(), mode)
	if _, err := output.WriteString(homeProbe + osc11BackgroundQuery + colorSchemeQuery); err != nil {
		return result
	}
	deadline := time.Now().Add(180 * time.Millisecond)
	var pending []byte
	gotGraphics := false
	for len(pending) < 4096 && inputReady(input, deadline) {
		var data [512]byte
		n, err := input.Read(data[:])
		if err != nil || n == 0 {
			break
		}
		pending = append(pending, data[:n]...)
		graphics, ok, rest := stripGraphicsReply(pending)
		if ok {
			result.graphics, gotGraphics = graphics, true
		}
		dark, ok, rest := extractOSC11Background(rest)
		if ok {
			result.dark, result.hasTheme = dark, true
		}
		pending, dark, ok = stripThemeNotifications(rest)
		if ok {
			result.dark, result.hasTheme = dark, true
		}
		if gotGraphics && result.hasTheme {
			break
		}
	}
	result.pending = pending
	return result
}
