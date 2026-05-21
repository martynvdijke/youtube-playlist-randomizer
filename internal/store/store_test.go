package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAndClose(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if s.db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestGetQuota_Default(t *testing.T) {
	s := newTestStore(t)
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Used != 0 {
		t.Errorf("expected Used=0, got %d", q.Used)
	}
	if q.Limit != DefaultQuotaLimit {
		t.Errorf("expected Limit=%d, got %d", DefaultQuotaLimit, q.Limit)
	}
	expected := UsableLimit(DefaultQuotaLimit)
	if q.Remaining != expected {
		t.Errorf("expected Remaining=%d (with 5%% safety margin), got %d", expected, q.Remaining)
	}
	if q.Date == "" {
		t.Error("expected non-empty Date")
	}
}

func TestAddQuota(t *testing.T) {
	s := newTestStore(t)
	usable := UsableLimit(DefaultQuotaLimit)
	q, err := s.AddQuota(5)
	if err != nil {
		t.Fatalf("AddQuota failed: %v", err)
	}
	if q.Used != 5 {
		t.Errorf("expected Used=5, got %d", q.Used)
	}
	if q.Remaining != usable-5 {
		t.Errorf("expected Remaining=%d, got %d", usable-5, q.Remaining)
	}

	q, err = s.AddQuota(10)
	if err != nil {
		t.Fatalf("AddQuota failed: %v", err)
	}
	if q.Used != 15 {
		t.Errorf("expected Used=15, got %d", q.Used)
	}
	if q.Remaining != usable-15 {
		t.Errorf("expected Remaining=%d, got %d", usable-15, q.Remaining)
	}
}

func TestAddQuota_Exhaustion(t *testing.T) {
	s := newTestStore(t)
	usable := UsableLimit(DefaultQuotaLimit)
	_, err := s.AddQuota(usable + 100)
	if err != nil {
		t.Fatalf("AddQuota failed: %v", err)
	}
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", q.Remaining)
	}
}

func TestSetQuotaLimit(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetQuotaLimit(500); err != nil {
		t.Fatalf("SetQuotaLimit failed: %v", err)
	}
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Limit != 500 {
		t.Errorf("expected Limit=500, got %d", q.Limit)
	}
}

func TestEstimateQuotaNeeded(t *testing.T) {
	s := newTestStore(t)
	tests := []struct {
		items    int
		expected int
	}{
		{0, 0},
		{1, QuotaCreatePlaylist + 1*QuotaInsertItem},
		{10, QuotaCreatePlaylist + 10*QuotaInsertItem},
		{100, QuotaCreatePlaylist + 100*QuotaInsertItem},
	}
	for _, tc := range tests {
		got := s.EstimateQuotaNeeded(tc.items)
		if got != tc.expected {
			t.Errorf("EstimateQuotaNeeded(%d) = %d, want %d", tc.items, got, tc.expected)
		}
	}
}

func TestCreateAndGetJob(t *testing.T) {
	s := newTestStore(t)
	err := s.CreateJob("job1", "pl123", "My Playlist", "My-randomized")
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	j, err := s.GetJob("job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.ID != "job1" {
		t.Errorf("expected ID 'job1', got '%s'", j.ID)
	}
	if j.SourcePlaylistID != "pl123" {
		t.Errorf("expected SourcePlaylistID 'pl123', got '%s'", j.SourcePlaylistID)
	}
	if j.NewName != "My-randomized" {
		t.Errorf("expected NewName 'My-randomized', got '%s'", j.NewName)
	}
	if j.Status != "pending" {
		t.Errorf("expected Status 'pending', got '%s'", j.Status)
	}
	if j.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if j.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}
	if j.UpdatedAt != j.CreatedAt {
		t.Errorf("expected UpdatedAt (%s) to equal CreatedAt (%s)", j.UpdatedAt, j.CreatedAt)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestUpdateJobStatus(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.UpdateJobStatus("job1", "fetching"); err != nil {
		t.Fatalf("UpdateJobStatus failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.Status != "fetching" {
		t.Errorf("expected Status 'fetching', got '%s'", j.Status)
	}
	if j.UpdatedAt < j.CreatedAt {
		t.Errorf("expected UpdatedAt (%s) to be >= CreatedAt (%s)", j.UpdatedAt, j.CreatedAt)
	}
}

func TestUpdateJobProgress(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.UpdateJobProgress("job1", 5, "new-pl-id"); err != nil {
		t.Fatalf("UpdateJobProgress failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.InsertedItems != 5 {
		t.Errorf("expected InsertedItems=5, got %d", j.InsertedItems)
	}
	if j.NewPlaylistID != "new-pl-id" {
		t.Errorf("expected NewPlaylistID 'new-pl-id', got '%s'", j.NewPlaylistID)
	}
}

func TestUpdateJobNewPlaylistID(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.UpdateJobNewPlaylistID("job1", "new-pl"); err != nil {
		t.Fatalf("UpdateJobNewPlaylistID failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.NewPlaylistID != "new-pl" {
		t.Errorf("expected NewPlaylistID 'new-pl', got '%s'", j.NewPlaylistID)
	}
}

func TestSetJobError(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.SetJobError("job1", "something went wrong"); err != nil {
		t.Fatalf("SetJobError failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.Status != "error" {
		t.Errorf("expected Status 'error', got '%s'", j.Status)
	}
	if j.Error != "something went wrong" {
		t.Errorf("expected Error 'something went wrong', got '%s'", j.Error)
	}
}

func TestSetJobDone(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.SetJobDone("job1"); err != nil {
		t.Fatalf("SetJobDone failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.Status != "done" {
		t.Errorf("expected Status 'done', got '%s'", j.Status)
	}
}

func TestSaveAndGetShuffledItems(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")

	items := []string{"vid1", "vid2", "vid3"}
	if err := s.SaveShuffledItems("job1", items); err != nil {
		t.Fatalf("SaveShuffledItems failed: %v", err)
	}

	uninserted, err := s.GetUninsertedItems("job1")
	if err != nil {
		t.Fatalf("GetUninsertedItems failed: %v", err)
	}
	if len(uninserted) != 3 {
		t.Fatalf("expected 3 uninserted items, got %d", len(uninserted))
	}
	for i, item := range uninserted {
		if item.VideoID != items[i] {
			t.Errorf("item %d: expected VideoID '%s', got '%s'", i, items[i], item.VideoID)
		}
		if item.Position != i {
			t.Errorf("item %d: expected Position %d, got %d", i, i, item.Position)
		}
	}
}

func TestMarkItemInserted(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	s.SaveShuffledItems("job1", []string{"vid1", "vid2", "vid3"})

	if err := s.MarkItemInserted("job1", 1); err != nil {
		t.Fatalf("MarkItemInserted failed: %v", err)
	}

	uninserted, _ := s.GetUninsertedItems("job1")
	if len(uninserted) != 2 {
		t.Fatalf("expected 2 uninserted items, got %d", len(uninserted))
	}
	if uninserted[0].VideoID != "vid1" {
		t.Errorf("expected first uninserted 'vid1', got '%s'", uninserted[0].VideoID)
	}
	if uninserted[1].VideoID != "vid3" {
		t.Errorf("expected second uninserted 'vid3', got '%s'", uninserted[1].VideoID)
	}
}

func TestGetPendingJobs(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "Playlist 1", "test1")
	s.CreateJob("job2", "pl2", "Playlist 2", "test2")
	s.SetJobDone("job2")

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pending job, got %d", len(jobs))
	}
	if jobs[0].ID != "job1" {
		t.Errorf("expected job ID 'job1', got '%s'", jobs[0].ID)
	}
}

func TestCreateJobWithExhaustedQuota(t *testing.T) {
	s := newTestStore(t)

	// Exhaust all quota
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	s.AddQuota(q.Remaining + 1)

	q, err = s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Remaining != 0 {
		t.Fatalf("expected 0 remaining quota, got %d", q.Remaining)
	}

	err = s.CreateJob("job1", "pl123", "My Playlist", "my-randomized")
	if err != nil {
		t.Fatalf("CreateJob failed with exhausted quota: %v", err)
	}

	j, err := s.GetJob("job1")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if j.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", j.Status)
	}
	if j.SourcePlaylistID != "pl123" {
		t.Errorf("expected SourcePlaylistID 'pl123', got '%s'", j.SourcePlaylistID)
	}
	if j.NewName != "my-randomized" {
		t.Errorf("expected NewName 'my-randomized', got '%s'", j.NewName)
	}
	if j.TotalItems != 0 {
		t.Errorf("expected TotalItems=0 for new job, got %d", j.TotalItems)
	}

	pending, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs failed: %v", err)
	}
	found := false
	for _, pj := range pending {
		if pj.ID == "job1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pending job to appear in GetPendingJobs results")
	}
}

func TestGetPendingJobs_IncludesZeroItemJobs(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test1")

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pending job, got %d", len(jobs))
	}
	if jobs[0].ID != "job1" {
		t.Errorf("expected job ID 'job1', got '%s'", jobs[0].ID)
	}
	if jobs[0].TotalItems != 0 {
		t.Errorf("expected TotalItems=0, got %d", jobs[0].TotalItems)
	}
}

func TestResumeJob(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	s.SaveShuffledItems("job1", []string{"vid1", "vid2"})

	items, err := s.ResumeJob("job1", "new-pl-id")
	if err != nil {
		t.Fatalf("ResumeJob failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	j, _ := s.GetJob("job1")
	if j.Status != "inserting" {
		t.Errorf("expected Status 'inserting', got '%s'", j.Status)
	}
	if j.NewPlaylistID != "new-pl-id" {
		t.Errorf("expected NewPlaylistID 'new-pl-id', got '%s'", j.NewPlaylistID)
	}
}

func TestGetLatestJob_None(t *testing.T) {
	s := newTestStore(t)
	j, err := s.GetLatestJob()
	if err != nil {
		t.Fatalf("GetLatestJob failed: %v", err)
	}
	if j != nil {
		t.Fatal("expected nil job")
	}
}

func TestGetLatestJob(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "First", "test1")
	s.CreateJob("job2", "pl2", "Second", "test2")

	j, err := s.GetLatestJob()
	if err != nil {
		t.Fatalf("GetLatestJob failed: %v", err)
	}
	if j == nil {
		t.Fatal("expected non-nil job")
	}
}

func TestPersistAcrossStores(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	s1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open s1 failed: %v", err)
	}
	s1.CreateJob("job1", "pl1", "", "test")
	s1.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open s2 failed: %v", err)
	}
	defer s2.Close()

	j, err := s2.GetJob("job1")
	if err != nil {
		t.Fatalf("GetJob from reopened store failed: %v", err)
	}
	if j.ID != "job1" {
		t.Errorf("expected ID 'job1', got '%s'", j.ID)
	}
}

func TestEnvironmentOverride(t *testing.T) {
	if DefaultQuotaLimit != 10000 {
		t.Errorf("expected DefaultQuotaLimit=10000, got %d", DefaultQuotaLimit)
	}
}

func TestEffectiveLimit(t *testing.T) {
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 0},
		{100, 95},
		{10000, 9500},
		{200, 190},
	}
	for _, tc := range tests {
		got := UsableLimit(tc.limit)
		if got != tc.expected {
			t.Errorf("UsableLimit(%d) = %d, want %d", tc.limit, got, tc.expected)
		}
	}
}

func TestGetQuota_WithSafetyMargin(t *testing.T) {
	s := newTestStore(t)
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	usable := UsableLimit(DefaultQuotaLimit)
	if q.Remaining != usable {
		t.Errorf("expected Remaining=%d (with 5%% safety margin), got %d", usable, q.Remaining)
	}
}

func TestGetQuota_UsedNearLimit(t *testing.T) {
	s := newTestStore(t)
	usable := UsableLimit(DefaultQuotaLimit)
	// Use up to exactly the usable limit
	s.AddQuota(usable)
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Remaining != 0 {
		t.Errorf("expected Remaining=0 (safety margin exhausted), got %d", q.Remaining)
	}
	if q.Used != usable {
		t.Errorf("expected Used=%d, got %d", usable, q.Used)
	}
}

func TestGetQuota_UsedBeyondLimit(t *testing.T) {
	s := newTestStore(t)
	usable := UsableLimit(DefaultQuotaLimit)
	// Use more than the usable limit but less than the hard limit
	s.AddQuota(usable + 200)
	q, err := s.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota failed: %v", err)
	}
	if q.Remaining != 0 {
		t.Errorf("expected Remaining=0 (over hard limit), got %d", q.Remaining)
	}
	if q.Limit != DefaultQuotaLimit {
		t.Errorf("expected Limit=%d, got %d", DefaultQuotaLimit, q.Limit)
	}
}

func TestSetJobPaused(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test")
	if err := s.SetJobPaused("job1"); err != nil {
		t.Fatalf("SetJobPaused failed: %v", err)
	}
	j, _ := s.GetJob("job1")
	if j.Status != "paused" {
		t.Errorf("expected Status 'paused', got '%s'", j.Status)
	}
	if j.PausedAt == "" {
		t.Error("expected non-empty PausedAt")
	}
}

func TestGetPendingJobs_IncludesPaused(t *testing.T) {
	s := newTestStore(t)
	s.CreateJob("job1", "pl1", "", "test1")
	s.SetJobPaused("job1")

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs failed: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.ID == "job1" {
			found = true
			if j.Status != "paused" {
				t.Errorf("expected status 'paused', got '%s'", j.Status)
			}
			if j.PausedAt == "" {
				t.Error("expected non-empty PausedAt")
			}
			break
		}
	}
	if !found {
		t.Error("expected paused job in GetPendingJobs results")
	}
}
