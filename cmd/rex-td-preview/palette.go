package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"rex-td/rex"
)

type palettePane struct {
	HostID    string `json:"host_id"`
	Host      string `json:"host"`
	SessionID string `json:"session_id"`
	Session   string `json:"session"`
	WindowID  string `json:"window_id"`
	BlockID   string `json:"block_id"`
	Title     string `json:"title"`
	Agent     string `json:"agent"`
}

type paletteTask struct {
	ID       int64         `json:"id"`
	Title    string        `json:"title"`
	State    string        `json:"state"`
	Archived bool          `json:"archived,omitempty"`
	Panes    []palettePane `json:"panes"`
}

type captureRequest struct {
	TaskID int64       `json:"task_id"`
	Title  string      `json:"title"`
	Pane   palettePane `json:"pane"`
}

func palette(args []string) error {
	if len(args) == 0 {
		return paletteError("specify a palette command", "需要指定命令面板操作")
	}
	path, err := taskDBPath()
	if err != nil {
		return err
	}
	store, err := openTaskStore(path)
	if err != nil {
		return err
	}
	defer store.db.Close()
	encode := json.NewEncoder(os.Stdout)
	if args[0] == "capture" {
		if len(args) != 2 {
			return paletteError("capture requires a pane", "缺少收录参数")
		}
		var request captureRequest
		if err := json.Unmarshal([]byte(args[1]), &request); err != nil {
			return err
		}
		taskID, err := store.captureLocalPane(request)
		if err != nil {
			return err
		}
		return encode.Encode(map[string]int64{"task_id": taskID})
	}
	if args[0] != "list" && args[0] != "find-list" {
		return handlePaletteNavigation(store, encode, args)
	}
	if len(args) != 1 {
		return paletteError("list accepts no additional arguments", "list 不接受额外参数")
	}
	tasks, err := store.load()
	if err != nil {
		return err
	}
	archived := make(map[int64]bool)
	for _, t := range tasks {
		if t.archived {
			archived[t.id] = true
		}
	}
	result := make([]paletteTask, 0, len(tasks))
	for _, t := range tasks {
		isArchived := t.archived || archived[t.parentID]
		if isArchived && args[0] == "list" {
			continue
		}
		state := storedStates[t.state]
		if t.state == paused {
			state = "Paused"
		}
		entry := paletteTask{ID: t.id, Title: t.title, State: state, Archived: isArchived, Panes: make([]palettePane, 0, len(t.panes))}
		for _, p := range t.panes {
			entry.Panes = append(entry.Panes, palettePane{
				p.Host.ID, p.Host.Label, p.SessionID, p.Session, p.WindowID, p.ID, paneTitle(p), p.App,
			})
		}
		result = append(result, entry)
	}
	return encode.Encode(result)
}

func (s *taskStore) captureLocalPane(request captureRequest) (int64, error) {
	p := request.Pane
	if p.HostID != "local" || !strings.HasPrefix(p.SessionID, "session:") || !strings.HasPrefix(p.BlockID, "block:") {
		return 0, paletteError("no local Rex pane found", "找不到本机 Rex 窗格")
	}
	pane := rex.Pane{
		Host:      rex.Host{ID: "local", Label: "本机"},
		SessionID: p.SessionID, Session: storedDisplay(p.Session), WindowID: p.WindowID,
		ID: p.BlockID, Label: storedDisplay(p.Title), App: storedDisplay(p.Agent),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	id := request.TaskID
	if id == 0 {
		title := strings.TrimSpace(storedDisplay(request.Title))
		if title == "" || len([]rune(title)) > 240 {
			return 0, paletteError("enter a title of 1 to 240 characters", "请填写 1 至 240 字的标题")
		}
		row, err := tx.ExecContext(ctx, "INSERT INTO task(id,title,state) VALUES("+nextStoredTaskID+",?,?)", title, storedStates[todo])
		if err != nil {
			return 0, err
		}
		id, err = row.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else if err := tx.QueryRowContext(ctx, "SELECT child.id FROM task child LEFT JOIN task parent ON parent.id=child.parent_id WHERE child.id=? AND child.archived=0 AND COALESCE(parent.archived,0)=0", id).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, paletteError("the task is no longer available", "任务已不存在，请重新查找")
		}
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, insertPaneBinding, paneBindingValues(id, pane)...); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

type paletteStoreTick struct{}

func (s *taskStore) changeVersion() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	var version int64
	err := s.db.QueryRowContext(ctx, "PRAGMA data_version").Scan(&version)
	return version, err
}

func (m *model) watchPaletteStore() tea.Cmd {
	if !m.focusPreview || m.store == nil {
		return nil
	}
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return paletteStoreTick{} })
}

func (m *model) refreshPaletteStore() bool {
	version, err := m.store.changeVersion()
	if err != nil || version == m.paletteStoreVersion {
		return false
	}
	tasks, err := m.store.load()
	if err != nil {
		return false
	}

	seen, err := m.store.loadSeenResults()
	if err != nil {
		return false
	}

	selected := m.selected()
	var selectedPane paneIdentity
	if selected != nil && m.detailSelection >= 0 && m.detailSelection < len(selected.panes) {
		selectedPane = identityOf(selected.panes[m.detailSelection])
	}
	previous := make(map[int64]*task, len(m.tasks))
	for _, t := range m.tasks {
		previous[t.id] = t
	}

	for i, t := range tasks {
		if existing := previous[t.id]; existing != nil {
			*existing = *t
			tasks[i] = existing
		}
	}
	m.taskSelection = clamp(m.taskSelection, 0, len(tasks)-1)
	if i := slices.Index(tasks, selected); i >= 0 {
		m.taskSelection = i
	}
	m.tasks = tasks
	if m.page == listPage && !m.attention {
		m.selectVisibleTask()
	}
	if task := m.selected(); task != nil {
		if i := slices.IndexFunc(task.panes, func(p rex.Pane) bool { return identityOf(p) == selectedPane }); i >= 0 {
			m.detailSelection = i
		}
		m.detailSelection = clamp(m.detailSelection, 0, len(task.panes)-1)
	}
	m.paletteStoreVersion = version
	m.seenResults = seen
	m.pruneSeenResults()
	m.refreshPanes()
	m.refreshCandidates()
	m.paletteResume, _ = m.store.readPaletteResume(os.Getenv("REX_CLIENT"))
	return true
}

func paletteError(en, zh string) error {
	if os.Getenv("REX_TD_LANG") == string(chinese) {
		return errors.New(zh)
	}
	return errors.New(en)
}

func runPalette(args []string) {
	if err := palette(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
