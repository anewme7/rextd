package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rex-td/rex"
)

type quotaWindow struct {
	Minutes int
	Used    float64
	Reset   int64
	Present bool
}

type quotaReading struct {
	Windows  [2]quotaWindow
	Received time.Time
}

func (q quotaReading) available() bool {
	return q.Windows[0].Present || q.Windows[1].Present
}

func quotaCachePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "rex-td", "claude-quota.json"), nil
}

func decodeClaudeStatus(input io.Reader) (quotaReading, error) {
	type limit struct {
		Used  *float64 `json:"used_percentage"`
		Reset int64    `json:"resets_at"`
	}
	var status struct {
		RateLimits struct {
			Five limit `json:"five_hour"`
			Week limit `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if err := json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&status); err != nil {
		return quotaReading{}, err
	}
	q := quotaReading{Received: time.Now()}
	for i, pair := range []limit{status.RateLimits.Five, status.RateLimits.Week} {
		if pair.Used != nil && *pair.Used >= 0 && *pair.Used <= 100 {
			q.Windows[i] = quotaWindow{[2]int{300, 10080}[i], *pair.Used, pair.Reset, true}
		}
	}
	return q, nil
}

func recordClaudeStatus(input io.Reader) error {
	q, err := decodeClaudeStatus(input)
	if err != nil || !q.available() {
		return err
	}
	path, err := quotaCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".claude-quota-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(q)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func claudeQuota() quotaReading {
	path, err := quotaCachePath()
	if err != nil {
		return quotaReading{}
	}
	file, err := os.Open(path)
	if err != nil {
		return quotaReading{}
	}
	defer file.Close()
	var q quotaReading
	if json.NewDecoder(io.LimitReader(file, 4096)).Decode(&q) != nil {
		return quotaReading{}
	}
	return q
}

// An explicit local Claude rejection can arrive before statusLine has any
// usage data. It applies only until the reset reported by Claude.
func claudeWeeklyLimit(panes []rex.Pane) quotaReading {
	var reading quotaReading
	for _, pane := range panes {
		if pane.Host.ID != "local" || pane.App != "claude-code" || pane.Status != "error" ||
			!pane.ReportedAt.After(reading.Received) {
			continue
		}
		if reset := claudeWeeklyReset(pane.Message, pane.ReportedAt); !reset.IsZero() {
			reading.Received = pane.ReportedAt
			reading.Windows[1] = quotaWindow{Minutes: quotaWeekMinutes, Used: 100, Reset: reset.Unix(), Present: true}
		}
	}
	return reading
}

func claudeWeeklyReset(message string, reported time.Time) time.Time {
	value, ok := strings.CutPrefix(message, "You've hit your weekly limit · resets ")
	if !ok {
		return time.Time{}
	}
	date, zone, ok := strings.Cut(value, " (")
	if !ok || !strings.HasSuffix(zone, ")") {
		return time.Time{}
	}
	location, err := time.LoadLocation(strings.TrimSuffix(zone, ")"))
	if err != nil {
		return time.Time{}
	}
	format := "2006 Jan 2 at 3pm"
	if strings.Contains(date, ":") {
		format = "2006 Jan 2 at 3:04pm"
	}
	year := reported.In(location).Year()
	reset, err := time.ParseInLocation(format, fmt.Sprintf("%d %s", year, date), location)
	if err != nil {
		return time.Time{}
	}
	if !reset.After(reported) {
		reset = reset.AddDate(1, 0, 0)
	}
	if reset.Sub(reported) > 8*24*time.Hour {
		return time.Time{}
	}
	return reset
}

func codexQuota(ctx context.Context) (quotaReading, error) {
	// An existing daemon already uses the user's configured account. Starting
	// a stdio instance is the documented fallback when it is not running.
	if q, err := codexQuotaVia(ctx, "app-server", "proxy"); err == nil {
		return q, nil
	}
	return codexQuotaVia(ctx, "app-server", "--listen", "stdio://")
}

type codexCall func(string, any) (json.RawMessage, error)

func codexQuotaVia(parent context.Context, args ...string) (quotaReading, error) {
	var q quotaReading
	err := withCodexRPC(parent, args, func(call codexCall) error {
		raw, err := call("account/rateLimits/read", nil)
		if err != nil {
			return err
		}
		q = decodeCodexQuota(raw)
		if !q.available() {
			return errors.New("Codex returned no supported rate-limit windows")
		}
		return nil
	})
	return q, err
}

func withCodexRPC(parent context.Context, args []string, read func(codexCall) error) error {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, agentProgram("codex"), args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return err
	}
	defer func() { in.Close(); cancel(); _ = cmd.Wait() }()

	enc := json.NewEncoder(in)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	id := 0
	call := func(method string, params any) (json.RawMessage, error) {
		id++
		request := map[string]any{"id": id, "method": method}
		if params != nil {
			request["params"] = params
		}
		if err := enc.Encode(request); err != nil {
			return nil, err
		}
		for scanner.Scan() {
			var r struct {
				ID     int
				Result json.RawMessage
				Error  *struct{ Code int }
			}
			if json.Unmarshal(scanner.Bytes(), &r) != nil || r.ID != id {
				continue
			}
			if r.Error != nil {
				return nil, fmt.Errorf("Codex App Server request failed (%d)", r.Error.Code)
			}
			return r.Result, nil
		}
		return nil, errors.New("Codex App Server did not respond")
	}
	if _, err := call("initialize", map[string]any{"clientInfo": map[string]string{
		"name": "rex_td", "title": "rex-td", "version": strings.TrimPrefix(version, "v"),
	}}); err != nil {
		return err
	}
	if err := enc.Encode(map[string]string{"method": "initialized"}); err != nil {
		return err
	}
	return read(call)
}

func decodeCodexQuota(data []byte) quotaReading {
	type window struct {
		Used    *float64 `json:"usedPercent"`
		Minutes int      `json:"windowDurationMins"`
		Reset   int64    `json:"resetsAt"`
	}
	type limits struct{ Primary, Secondary window }
	var result struct {
		RateLimits limits            `json:"rateLimits"`
		ByID       map[string]limits `json:"rateLimitsByLimitId"`
	}
	if json.Unmarshal(data, &result) != nil {
		return quotaReading{}
	}
	set := result.RateLimits
	if specific, ok := result.ByID["codex"]; ok {
		set = specific
	}
	q := quotaReading{Received: time.Now()}
	for i, w := range []window{set.Primary, set.Secondary} {
		if w.Used != nil && *w.Used >= 0 && *w.Used <= 100 {
			q.Windows[i] = quotaWindow{w.Minutes, *w.Used, w.Reset, true}
		}
	}
	return q
}
