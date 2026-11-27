package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"modernc.org/sqlite"
)

var cancelledReadTestID atomic.Uint64

func TestCancelledReadDoesNotBlockCreatingATask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	name := fmt.Sprintf("rextd_wait_on_read_%d", cancelledReadTestID.Add(1))
	started := make(chan struct{})
	sqlite.MustRegisterDeterministicScalarFunction(name, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		close(started)
		<-ctx.Done()
		// The cancelled query already has a row when it returns to the driver.
		time.Sleep(20 * time.Millisecond)
		return int64(1), nil
	})
	store, err := openTaskStore(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.db.Close() })
	if _, err := store.addTask("Existing task"); err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	go func() {
		var value int64
		read <- store.db.QueryRowContext(ctx, "SELECT "+name+"() FROM task").Scan(&value)
	}()
	select {
	case <-started:
	case err := <-read:
		t.Fatalf("read ended before cancellation: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("read did not start")
	}
	cancel()
	if err := <-read; !errors.Is(err, context.Canceled) {
		t.Fatalf("read = %v, want context canceled", err)
	}
	if _, err := store.addTask("Saved after cancellation"); err != nil {
		t.Fatalf("cancelled read blocked task creation: %v", err)
	}
}
