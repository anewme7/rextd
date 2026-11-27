// rextd runs the Go sidebar in a terminal or a Rex pane.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"rex-td/rex"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "agent-details" {
		if err := runAgentDetails(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "palette" {
		runPalette(os.Args[2:])
		return
	}
	demo := flag.Bool("demo", false, "show sample tasks and metrics")
	host := flag.Int("host", -1, "open Alerts for host 1 to 9 (0 shows all hosts)")
	startHome := flag.Bool("home", false, "open the task board home")
	defaultLanguage := english
	if os.Getenv("REX_TD_LANG") == string(chinese) {
		defaultLanguage = chinese
	}
	selectedLanguage := flag.String("lang", string(defaultLanguage), "interface language (en or zh)")
	showVersion := flag.Bool("version", false, "print the build version")
	cpu := flag.Bool("cpu-snapshot", false, "print native CPU counters as JSON")
	claude := flag.Bool("claude-statusline", false, "read Claude Code status-line JSON from stdin")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [--lang en|zh] [--demo | --home | --host 0..9 | --version | --cpu-snapshot | --claude-statusline]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 || *host < -1 || *host > 9 {
		flag.Usage()
		os.Exit(2)
	}
	uiLanguage := language(*selectedLanguage)
	if uiLanguage != english && uiLanguage != chinese {
		fmt.Fprintln(os.Stderr, defaultLanguage.text("无效的界面语言，请使用 --lang en 或 --lang zh。"))
		os.Exit(2)
	}

	var err error
	switch {
	case *showVersion:
		fmt.Println("rextd", version)
	case *cpu:
		err = printCPUSnapshot(os.Stdout)
	case *claude:
		err = recordClaudeStatus(os.Stdin)
	default:
		err = run(*demo, *startHome, *host, uiLanguage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, uiLanguage.text("无法启动 rextd")+":", err)
		os.Exit(1)
	}
}

func run(demo, startHome bool, host int, uiLanguage language) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan os.Signal, 1)
	signal.Notify(requests, syscall.SIGUSR1)
	defer signal.Stop(requests)

	logger := previewLogger()
	logger.Info("rex-td started", "demo", demo)
	defer logger.Info("rex-td stopped")

	m := newModel(demo)
	m.language, m.focusPreview, m.ctx = uiLanguage, true, ctx
	m.genericDirs = genericDirectories(os.Getenv("REX_TD_GENERIC_DIRS"))
	if demo {
		m.switchHomeTab(nowTab)
	}
	if !demo {
		path, err := taskDBPath()
		if err != nil {
			return err
		}
		m.store, err = openTaskStore(path)
		if err != nil {
			return err
		}
		defer m.store.db.Close()
		m.paletteStoreVersion, _ = m.store.changeVersion()
		m.tasks, err = m.store.load()
		if err != nil {
			return err
		}
		m.switchHomeTab(nowTab)
		m.paletteResume, _ = m.store.readPaletteResume(os.Getenv("REX_CLIENT"))
		if startHome || host >= 0 {
			m.paletteResumeApplied = m.paletteResume.revision
		} else {
			m.applyPaletteResume()
		}
		m.candidateIgnored, err = m.store.loadCandidateIgnores()
		if err != nil {
			return err
		}
		m.seenResults, err = m.store.loadSeenResults()
		if err != nil {
			return err
		}
		m.candidateWatches, err = m.store.loadCandidateWatches()
		if err != nil {
			return err
		}
		defer func() {
			cancel()
			m.metricWorkers.Wait()
		}()
	}
	if !demo {
		current, err := currentRexContext(ctx)
		m.rexContext = current
		if err != nil {
			logger.Warn("Rex pane location unavailable", "error", err)
		}
		if current.Inside() {
			m.previews, err = rex.NewTerminalPreviews()
			if err != nil {
				logger.Warn("Could not reuse Rex preview connections", "error", err)
			}
			var done <-chan struct{}
			m.updates, done = current.WatchFleet(ctx, logger, m.previews)
			defer func() {
				cancel()
				<-done
				m.previews.Close()
			}()
		}
	}
	if host >= 0 {
		m.showHost(host)
		if host > len(m.feed.Hosts) {
			m.startupHost = host
		}
	}
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithFPS(24)}
	termOptions, input := m.homeTerminalOptions()
	options = append(options, termOptions...)
	program := tea.NewProgram(m, options...)
	if input != nil {
		input.attachProgram(program)
	}
	if term.IsTerminal(os.Stdout.Fd()) {
		_, _ = os.Stdout.WriteString(colorSchemeNotifyOn)
		defer os.Stdout.WriteString(colorSchemeNotifyOff)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-requests:
				program.Send(openHome{})
			}
		}
	}()
	_, err := program.Run()
	return err
}
