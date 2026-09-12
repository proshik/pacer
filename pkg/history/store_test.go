package history

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	return store
}

var morning = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)

func TestSavedRunsComeBackNewestFirst(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	half := time.Hour + 38*time.Minute + 48*time.Second

	if _, err := store.Save(ctx, 42, 10000, 50*time.Minute, morning); err != nil {
		t.Fatalf("save 10 km: %v", err)
	}
	if _, err := store.Save(ctx, 42, 21097, half, morning.Add(time.Hour)); err != nil {
		t.Fatalf("save half: %v", err)
	}

	runs, err := store.List(ctx, 42, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}

	newest := runs[0]
	if newest.Distance != 21097 || newest.Time != half || !newest.SavedAt.Equal(morning.Add(time.Hour)) {
		t.Errorf("newest = %+v, want the half marathon saved at %v", newest, morning.Add(time.Hour))
	}
	if runs[1].Distance != 10000 || runs[1].Time != 50*time.Minute {
		t.Errorf("oldest = %+v, want 10 km in 50:00", runs[1])
	}
	if newest.ID == runs[1].ID {
		t.Errorf("both runs share id %d", newest.ID)
	}
}

func TestRunsAreIsolatedPerUser(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	if _, err := store.Save(ctx, 42, 10000, 50*time.Minute, morning); err != nil {
		t.Fatalf("save: %v", err)
	}

	runs, err := store.List(ctx, 7, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(runs) != 0 {
		t.Errorf("another user sees %d runs, want 0", len(runs))
	}
}

func TestListRespectsLimit(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	for i := range 3 {
		if _, err := store.Save(ctx, 42, 5000*(i+1), time.Duration(i+1)*20*time.Minute, morning.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	runs, err := store.List(ctx, 42, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(runs) != 2 || runs[0].Distance != 15000 || runs[1].Distance != 10000 {
		t.Errorf("runs = %+v, want the two newest: 15 km then 10 km", runs)
	}
}

func TestDeleteRemovesOnlyTheOwnersRun(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	run, err := store.Save(ctx, 42, 10000, 50*time.Minute, morning)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.Delete(ctx, 7, run.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete by another user: err = %v, want %v", err, ErrNotFound)
	}
	if runs, _ := store.List(ctx, 42, 10); len(runs) != 1 {
		t.Fatalf("owner's run is gone after another user's delete")
	}

	if err := store.Delete(ctx, 42, run.ID); err != nil {
		t.Fatalf("delete by owner: %v", err)
	}
	if runs, _ := store.List(ctx, 42, 10); len(runs) != 0 {
		t.Errorf("run still listed after the owner deleted it")
	}
}

func TestSaveRejectsNonsense(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	if _, err := store.Save(ctx, 42, 0, time.Hour, morning); !errors.Is(err, ErrInvalidRun) {
		t.Errorf("zero distance: err = %v, want %v", err, ErrInvalidRun)
	}
	if _, err := store.Save(ctx, 42, 10000, 0, morning); !errors.Is(err, ErrInvalidRun) {
		t.Errorf("zero time: err = %v, want %v", err, ErrInvalidRun)
	}
}

func TestStoreSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	ctx := context.Background()

	first, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := first.Save(ctx, 42, 10000, 50*time.Minute, morning); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Opening an existing file must not recreate or wipe the schema.
	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = second.Close() }()

	if runs, err := second.List(ctx, 42, 10); err != nil || len(runs) != 1 {
		t.Errorf("after reopen: %d runs, err %v; want 1 run", len(runs), err)
	}
}

// The HTTP server saves from many goroutines at once, which is exactly where
// SQLite reports "database is locked" unless the store serializes writers.
func TestConcurrentSavesAllLand(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)

	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := store.Save(ctx, 42, 1000+i, 5*time.Minute, morning.Add(time.Duration(i)*time.Second)); err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent save: %v", err)
	}

	if runs, err := store.List(ctx, 42, 100); err != nil || len(runs) != writers {
		t.Errorf("stored %d runs, err %v; want %d", len(runs), err, writers)
	}
}
