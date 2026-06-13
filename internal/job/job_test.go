package job

import (
	"path/filepath"
	"testing"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

func TestProgressSnapshot(t *testing.T) {
	p := &Progress{}
	p.mu.Lock()
	p.Status = StatusPending
	p.Progress = 50
	p.Total = 100
	p.Done = 25
	p.NewPlaylistID = "pl-new"
	p.Error = ""
	p.mu.Unlock()

	snap := p.Snapshot()
	if snap.Status != StatusPending {
		t.Errorf("expected Status=%s, got %s", StatusPending, snap.Status)
	}
	if snap.Progress != 50 {
		t.Errorf("expected Progress=50, got %d", snap.Progress)
	}
	if snap.Total != 100 {
		t.Errorf("expected Total=100, got %d", snap.Total)
	}
	if snap.Done != 25 {
		t.Errorf("expected Done=25, got %d", snap.Done)
	}
	if snap.NewPlaylistID != "pl-new" {
		t.Errorf("expected NewPlaylistID='pl-new', got %q", snap.NewPlaylistID)
	}
}

func TestProgressSnapshot_Empty(t *testing.T) {
	p := &Progress{}
	snap := p.Snapshot()
	if snap.Status != "" {
		t.Errorf("expected empty Status, got %q", snap.Status)
	}
	if snap.Total != 0 {
		t.Errorf("expected Total=0, got %d", snap.Total)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	p := &Progress{}
	p.mu.Lock()
	p.Status = StatusPending
	p.mu.Unlock()

	snap := p.Snapshot()
	snap.Status = StatusDone
	snap.Total = 999

	p.mu.RLock()
	if p.Status != StatusPending {
		t.Errorf("original Status changed to %q", p.Status)
	}
	if p.Total != 0 {
		t.Errorf("original Total changed to %d", p.Total)
	}
	p.mu.RUnlock()
}

func TestNewRunner(t *testing.T) {
	r := New(nil, nil, nil, nil, nil)
	if r == nil {
		t.Fatal("expected non-nil Runner")
	}
}

func TestGetProgress_NotFound(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	r := New(s, nil, nil, nil, nil)
	p := r.GetProgress("nonexistent")
	if p != nil {
		t.Errorf("expected nil, got %v", p)
	}
}

func TestGetProgress_Found(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.CreateJob("job1", "pl1", "", "test"); err != nil {
		t.Fatalf("create job: %v", err)
	}

	r := New(s, nil, nil, nil, nil)
	p := r.GetProgress("job1")
	if p == nil {
		t.Fatal("expected non-nil progress")
	}
	if p.Status != StatusPending {
		t.Errorf("expected StatusPending, got %s", p.Status)
	}
}
