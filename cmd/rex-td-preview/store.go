package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"

	"rex-td/rex"
)

// Keep the legacy SQLite token for paused tasks so existing databases and
// earlier builds can still read the same records.
var storedStates = [...]string{"To do", "In progress", "Blocked", "Done", "Cancelled"}
var storedColors = [...]string{"", "blue", "green", "amber", "violet"}

type taskStore struct{ db *sql.DB }

func taskDBPath() (string, error) {
	dir := os.Getenv("REX_TD_DATA_DIR")
	if !filepath.IsAbs(dir) {
		base := os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(base) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "rex-td")
	}
	return filepath.Join(dir, "tasks.sqlite3"), nil
}

func openTaskStore(path string) (*taskStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	location := url.URL{Scheme: "file", Path: path, RawQuery: "_pragma=foreign_keys%3DON&_pragma=busy_timeout%3D1200&_txlock=immediate"}
	db, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &taskStore{db: db}
	if err := s.initialize(path); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *taskStore) initialize(path string) error {
	// Incremental auto-vacuum is selected before any table exists. An
	// existing database keeps its original settings and reusable free pages.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if info, err := os.Stat(path); err == nil && info.Size() == 0 {
		if _, err := s.db.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, taskSchema); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"session_title", "TEXT NOT NULL DEFAULT ''"},
		{"agent_app", "TEXT NOT NULL DEFAULT ''"},
		{"source", "TEXT NOT NULL DEFAULT 'manual'"},
		{"linked_at", "INTEGER NOT NULL DEFAULT 0"},
	} {
		var exists bool
		err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pragma_table_info('rex_pane_binding') WHERE name=?)", column.name).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE rex_pane_binding ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return err
			}
		}
	}
	for _, column := range []struct{ name, definition string }{
		{"previous_state", "TEXT NOT NULL DEFAULT 'To do'"},
		{"archived", "INTEGER NOT NULL DEFAULT 0"},
		{"sort_order", "INTEGER NOT NULL DEFAULT 0"},
		{"parent_id", "INTEGER REFERENCES task(id)"},
	} {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pragma_table_info('task') WHERE name=?)", column.name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE task ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

const taskSchema = `CREATE TABLE IF NOT EXISTS task(id INTEGER PRIMARY KEY, title TEXT NOT NULL, state TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '', review INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS task_completion(seq INTEGER PRIMARY KEY, task INTEGER UNIQUE NOT NULL REFERENCES task(id) ON DELETE CASCADE);
CREATE TRIGGER IF NOT EXISTS rextd_task_completed AFTER UPDATE OF state ON task
WHEN NEW.state='Done' AND OLD.state<>NEW.state BEGIN
    INSERT OR REPLACE INTO task_completion(task) VALUES(NEW.id);
END;
CREATE TABLE IF NOT EXISTS task_id_floor(
    slot INTEGER PRIMARY KEY CHECK(slot=1), deleted_id INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS palette_resume(
    slot INTEGER PRIMARY KEY CHECK(slot=1), revision INTEGER NOT NULL,
    task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, reason TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS palette_client_resume(
    client TEXT PRIMARY KEY, revision INTEGER NOT NULL,
    task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, reason TEXT NOT NULL) WITHOUT ROWID;
CREATE TRIGGER IF NOT EXISTS palette_client_resume_limit AFTER INSERT ON palette_client_resume
WHEN NEW.client<>'' BEGIN
    DELETE FROM palette_client_resume WHERE client IN (
        SELECT client FROM palette_client_resume WHERE client<>'' ORDER BY revision DESC LIMIT -1 OFFSET 128
    );
END;
INSERT OR IGNORE INTO palette_client_resume(client,revision,task,host_id,session_id,block_id,reason)
    SELECT '',revision,task,host_id,session_id,block_id,reason FROM palette_resume WHERE slot=1;
CREATE TABLE IF NOT EXISTS seen_pane_result(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL, reported_at INTEGER NOT NULL,
    PRIMARY KEY(host_id, session_id, block_id)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS task_pin(task INTEGER PRIMARY KEY REFERENCES task(id));
CREATE TABLE IF NOT EXISTS task_color(task INTEGER PRIMARY KEY REFERENCES task(id), color TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS task_kind(task INTEGER PRIMARY KEY REFERENCES task(id) ON DELETE CASCADE, kind INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS result(id INTEGER PRIMARY KEY, task INTEGER NOT NULL, body TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS rex_td_result_task ON result(task,id);
CREATE TABLE IF NOT EXISTS pane_link(task INTEGER, session_id TEXT, block_id TEXT, label TEXT,
    PRIMARY KEY(task,session_id,block_id));
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS pane_agent_binding(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, app TEXT NOT NULL,
    agent_id TEXT NOT NULL, pid INTEGER NOT NULL,
    PRIMARY KEY(host_id,session_id,block_id,app));
CREATE TABLE IF NOT EXISTS candidate_ignore(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL, topic TEXT NOT NULL,
    PRIMARY KEY(host_id, session_id, block_id));
CREATE TABLE IF NOT EXISTS candidate_watch(
    host_id TEXT NOT NULL, session_id TEXT NOT NULL, block_id TEXT NOT NULL,
    PRIMARY KEY(host_id, session_id, block_id));
CREATE TABLE IF NOT EXISTS rex_pane_binding(
    id INTEGER PRIMARY KEY, task INTEGER NOT NULL REFERENCES task(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL, host_label TEXT NOT NULL, server TEXT NOT NULL,
    session_id TEXT NOT NULL, window_id TEXT NOT NULL, block_id TEXT NOT NULL, title TEXT NOT NULL,
    UNIQUE(task,host_id,session_id,block_id));
INSERT OR IGNORE INTO rex_pane_binding(task,host_id,host_label,server,session_id,window_id,block_id,title)
SELECT p.task,'local','本机','',p.session_id,'',p.block_id,p.label
FROM pane_link AS p JOIN task AS t ON t.id=p.task
WHERE NOT EXISTS (SELECT 1 FROM meta WHERE key='go-pane-binding-v1')
ORDER BY p.rowid;
INSERT OR IGNORE INTO meta(key) VALUES ('go-pane-binding-v1');`

func (s *taskStore) exec(statement string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, statement, args...)
	return err
}

func (s *taskStore) load() ([]*task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,t.title,t.note,t.state,t.previous_state,t.archived,t.sort_order,COALESCE(t.parent_id,0),COALESCE(completion.seq,0),
    (p.task IS NOT NULL) AS pinned, COALESCE(c.color,'') AS color, COALESCE(k.kind,0),
    COALESCE((SELECT body FROM result r WHERE r.task=t.id ORDER BY r.id DESC LIMIT 1),'') AS result,
    (SELECT count(*) FROM result r WHERE r.task=t.id) AS results,
    json(COALESCE((SELECT json_group_array(json_object(
        'host_id',b.host_id,'host_label',b.host_label,'server',b.server,'session_id',b.session_id,
        'window_id',b.window_id,'block_id',b.block_id,'title',b.title,
        'session_title',b.session_title,'agent_app',b.agent_app,'source',b.source,'linked_at',b.linked_at))
        FROM (SELECT * FROM rex_pane_binding WHERE task=t.id ORDER BY id) b),'[]')) AS bindings
FROM task t LEFT JOIN task_completion completion ON completion.task=t.id LEFT JOIN task_pin p ON p.task=t.id LEFT JOIN task_color c ON c.task=t.id
LEFT JOIN task_kind k ON k.task=t.id
ORDER BY (t.parent_id IS NULL AND pinned) DESC,COALESCE(NULLIF(t.sort_order,0),t.id),t.id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*task
	for rows.Next() {
		var title, note, state, previousState, color, result, links string
		t := &task{}
		if err := rows.Scan(&t.id, &title, &note, &state, &previousState, &t.archived, &t.sortOrder, &t.parentID, &t.completionOrder, &t.pinned, &color, &t.kind, &result, &t.resultCount, &links); err != nil {
			return nil, err
		}
		t.title, t.note, t.result = storedDisplay(title), storedDisplay(note), storedDisplay(result)
		t.state = taskState(max(0, slices.Index(storedStates[:], state)))
		t.previousState = taskState(max(0, slices.Index(storedStates[:done], previousState)))
		t.color = max(0, slices.Index(storedColors[:], color))
		if t.kind < uncategorized || int(t.kind) >= len(kindLabels) {
			t.kind = uncategorized
		}
		var bindings []struct {
			HostID       string `json:"host_id"`
			HostLabel    string `json:"host_label"`
			Server       string
			SessionID    string `json:"session_id"`
			WindowID     string `json:"window_id"`
			BlockID      string `json:"block_id"`
			Title        string
			SessionTitle string `json:"session_title"`
			AgentApp     string `json:"agent_app"`
			Source       linkSource
			LinkedAt     int64 `json:"linked_at"`
		}
		if err := json.Unmarshal([]byte(links), &bindings); err != nil {
			return nil, err
		}
		for _, b := range bindings {
			t.addBinding(rex.Pane{
				ID: b.BlockID, SessionID: b.SessionID, WindowID: b.WindowID, Label: storedDisplay(b.Title),
				Session: storedDisplay(b.SessionTitle), App: storedDisplay(b.AgentApp),
				Host: rex.Host{ID: b.HostID, Label: storedDisplay(b.HostLabel), Server: b.Server},
			}, paneBinding{source: b.Source, linkedAt: b.LinkedAt})
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func storedDisplay(value string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, value)
}

func (s *taskStore) addTask(title string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var id int64
	err := s.db.QueryRowContext(ctx, "INSERT INTO task(id,title,state) VALUES ("+nextStoredTaskID+",?,'To do') RETURNING id", title).Scan(&id)
	return id, err
}

// IDs held by an open board must never identify a new task after deletion.
const nextStoredTaskID = `max(COALESCE((SELECT max(id) FROM task),0),
    COALESCE((SELECT deleted_id FROM task_id_floor WHERE slot=1),0))+1`

func (s *taskStore) pin(id int64, value bool) error {
	if value {
		return s.exec("INSERT OR IGNORE INTO task_pin(task) VALUES(?)", id)
	}
	return s.exec("DELETE FROM task_pin WHERE task=?", id)
}

func (s *taskStore) color(id int64, color string) error {
	if color == "" {
		return s.exec("DELETE FROM task_color WHERE task=?", id)
	}
	return s.exec("INSERT INTO task_color(task,color) VALUES(?,?) ON CONFLICT(task) DO UPDATE SET color=excluded.color", id, color)
}

func (s *taskStore) state(id int64, value taskState) error {
	return s.exec("UPDATE task SET state=? WHERE id=?", storedStates[value], id)
}

func (s *taskStore) setKind(id int64, kind taskKind) error {
	if kind == uncategorized {
		return s.exec("DELETE FROM task_kind WHERE task=?", id)
	}
	return s.exec("INSERT INTO task_kind(task,kind) VALUES(?,?) ON CONFLICT(task) DO UPDATE SET kind=excluded.kind", id, kind)
}

func (s *taskStore) bind(id int64, pane rex.Pane, remove bool) error {
	if remove {
		if err := s.exec("DELETE FROM rex_pane_binding WHERE task=? AND host_id=? AND session_id=? AND block_id=?",
			id, pane.Host.ID, pane.SessionID, pane.ID); err != nil {
			return err
		}
		// A busy database can reuse its free pages until the next unlink.
		_ = s.exec("PRAGMA incremental_vacuum(8)")
		return nil
	}
	return s.exec(insertPaneBinding, paneBindingValues(id, pane)...)
}

const paneBindingColumns = `rex_pane_binding(task,host_id,host_label,server,session_id,window_id,block_id,title,session_title,agent_app,source,linked_at)`

const insertPaneBinding = `INSERT INTO ` + paneBindingColumns + `
VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(task,host_id,session_id,block_id) DO NOTHING`

func paneBindingValues(id int64, pane rex.Pane) []any {
	return sourcedPaneBindingValues(id, pane, paneBinding{source: linkManual, linkedAt: time.Now().UnixMilli()})
}

func sourcedPaneBindingValues(id int64, pane rex.Pane, binding paneBinding) []any {
	return []any{id, pane.Host.ID, pane.Host.Label, pane.Host.Server, pane.SessionID, pane.WindowID, pane.ID, paneTitle(pane), pane.Session, pane.App, binding.source, binding.linkedAt}
}

// Check the current database under the same statement as the insert so two
// open boards cannot automatically attach the same pane to different tasks.
func (s *taskStore) bindNewPane(id int64, pane rex.Pane, binding paneBinding, activeOnly bool) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `INSERT INTO `+paneBindingColumns+`
SELECT ?,?,?,?,?,?,?,?,?,?,?,?
WHERE EXISTS (SELECT 1 FROM task t LEFT JOIN task parent ON parent.id=t.parent_id
              WHERE t.id=? AND (NOT ? OR t.state=?) AND t.archived=0 AND COALESCE(parent.archived,0)=0)
  AND NOT EXISTS (SELECT 1 FROM rex_pane_binding WHERE host_id=? AND session_id=? AND block_id=?)`,
		append(sourcedPaneBindingValues(id, pane, binding), id, activeOnly, storedStates[working], pane.Host.ID, pane.SessionID, pane.ID)...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// A move keeps the old binding if the destination no longer accepts links.
func (s *taskStore) movePaneBinding(from, to int64, pane rex.Pane, binding paneBinding) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM rex_pane_binding WHERE task=? AND host_id=? AND session_id=? AND block_id=?",
		from, pane.Host.ID, pane.SessionID, pane.ID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return err
		}
		return errPaneLinkChanged
	}
	if to != 0 {
		var available bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM task t LEFT JOIN task parent ON parent.id=t.parent_id
			WHERE t.id=? AND t.archived=0 AND COALESCE(parent.archived,0)=0)`, to).Scan(&available)
		if err != nil {
			return err
		}
		if !available {
			return errPaneLinkChanged
		}
		if _, err := tx.ExecContext(ctx, insertPaneBinding, sourcedPaneBindingValues(to, pane, binding)...); err != nil {
			return err
		}
	}
	return tx.Commit()
}
