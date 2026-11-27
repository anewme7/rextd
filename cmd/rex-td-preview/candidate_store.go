package main

import (
	"context"
	"time"

	"rex-td/rex"
)

func (s *taskStore) loadCandidateWatches() (map[paneIdentity]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT host_id, session_id, block_id FROM candidate_watch")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	watches := make(map[paneIdentity]bool)
	for rows.Next() {
		var id paneIdentity
		if err := rows.Scan(&id.host, &id.session, &id.block); err != nil {
			return nil, err
		}
		watches[id] = true
	}
	return watches, rows.Err()
}

func (s *taskStore) setCandidateWatch(id paneIdentity, value bool) error {
	if value {
		return s.exec("INSERT OR IGNORE INTO candidate_watch(host_id,session_id,block_id) VALUES(?,?,?)",
			id.host, id.session, id.block)
	}
	return s.exec("DELETE FROM candidate_watch WHERE host_id=? AND session_id=? AND block_id=?",
		id.host, id.session, id.block)
}

func (s *taskStore) loadCandidateIgnores() (map[paneIdentity]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, "SELECT host_id, session_id, block_id, topic FROM candidate_ignore")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ignored := make(map[paneIdentity]string)
	for rows.Next() {
		var id paneIdentity
		var topic string
		if err := rows.Scan(&id.host, &id.session, &id.block, &topic); err != nil {
			return nil, err
		}
		ignored[id] = topic
	}
	return ignored, rows.Err()
}

func (s *taskStore) ignoreCandidate(id paneIdentity, topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO candidate_ignore(host_id,session_id,block_id,topic) VALUES(?,?,?,?)",
		id.host, id.session, id.block, topic); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM candidate_ignore WHERE rowid IN (SELECT rowid FROM candidate_ignore ORDER BY rowid DESC LIMIT -1 OFFSET 200)"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *taskStore) addCandidateTask(task *task, pane rex.Pane) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO task(id,title,state,note)
SELECT `+nextStoredTaskID+`, ?, 'To do', ? WHERE NOT EXISTS (
    SELECT 1 FROM rex_pane_binding WHERE host_id=? AND session_id=? AND block_id=?
) RETURNING id`, task.title, task.note, pane.Host.ID, pane.SessionID, pane.ID).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, insertPaneBinding, paneBindingValues(id, pane)...); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
