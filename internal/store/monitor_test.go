package store

import (
	"testing"
	"time"

	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/model"
)

// --- Monitor CRUD ---

func TestCreateMonitor_Defaults(t *testing.T) {
	db := testDB(t)

	m := &Monitor{
		Name:     "Test Monitor",
		Platform: "netease",
		ChartID:  "19723756",
	}
	if err := CreateMonitor(db, m); err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}

	if m.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}
	if m.TopN != 20 {
		t.Errorf("TopN default: got %d, want 20", m.TopN)
	}
	if m.Interval != 12 {
		t.Errorf("Interval default: got %d, want 12", m.Interval)
	}
	if m.NextRunAt.IsZero() {
		t.Error("NextRunAt should be set")
	}
}

func TestCreateMonitor_ClampTopN(t *testing.T) {
	db := testDB(t)

	m := &Monitor{
		Name:     "High TopN",
		Platform: "qq",
		ChartID:  "26",
		TopN:     200,
		Interval: 6,
	}
	if err := CreateMonitor(db, m); err != nil {
		t.Fatal(err)
	}
	if m.TopN != 100 {
		t.Errorf("TopN should be clamped to 100, got %d", m.TopN)
	}
}

func TestCreateMonitor_InvalidInterval(t *testing.T) {
	db := testDB(t)

	m := &Monitor{
		Name:     "Bad Interval",
		Platform: "kugou",
		ChartID:  "6666",
		TopN:     10,
		Interval: 8, // not 6/12/24
	}
	if err := CreateMonitor(db, m); err != nil {
		t.Fatal(err)
	}
	if m.Interval != 12 {
		t.Errorf("Interval should default to 12, got %d", m.Interval)
	}
}

func TestGetMonitor(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Get Test", Platform: "netease", ChartID: "3778678", Interval: 6}
	_ = CreateMonitor(db, m)

	got, err := GetMonitor(db, m.ID)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if got.Name != "Get Test" {
		t.Errorf("Name: got %q", got.Name)
	}
	if got.Platform != "netease" {
		t.Errorf("Platform: got %q", got.Platform)
	}
}

func TestGetMonitor_NotFound(t *testing.T) {
	db := testDB(t)
	_, err := GetMonitor(db, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent monitor")
	}
}

func TestUpdateMonitor(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Update Me", Platform: "qq", ChartID: "27", Interval: 6, TopN: 10}
	_ = CreateMonitor(db, m)

	m.Name = "Updated"
	m.TopN = 50
	m.Interval = 24
	if err := UpdateMonitor(db, m); err != nil {
		t.Fatal(err)
	}

	got, _ := GetMonitor(db, m.ID)
	if got.Name != "Updated" {
		t.Errorf("Name: got %q", got.Name)
	}
	if got.TopN != 50 {
		t.Errorf("TopN: got %d", got.TopN)
	}
	if got.Interval != 24 {
		t.Errorf("Interval: got %d", got.Interval)
	}
}

func TestDeleteMonitor_CascadesRuns(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Delete Me", Platform: "kugou", ChartID: "8888", Interval: 12}
	_ = CreateMonitor(db, m)

	// Create some runs.
	for i := 0; i < 3; i++ {
		_ = CreateMonitorRun(db, &MonitorRun{
			MonitorID: m.ID,
			StartedAt: time.Now(),
			Status:    "done",
		})
	}

	// Verify runs exist.
	runs, _ := ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs before delete, got %d", len(runs))
	}

	if err := DeleteMonitor(db, m.ID); err != nil {
		t.Fatal(err)
	}

	// Monitor should be gone.
	_, err := GetMonitor(db, m.ID)
	if err == nil {
		t.Error("monitor should be deleted")
	}

	// Runs should be gone too.
	runs, _ = ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete, got %d", len(runs))
	}
}

func TestListMonitors(t *testing.T) {
	db := testDB(t)

	_ = CreateMonitor(db, &Monitor{Name: "A", Platform: "netease", ChartID: "1", Interval: 6})
	_ = CreateMonitor(db, &Monitor{Name: "B", Platform: "qq", ChartID: "2", Interval: 12})

	monitors, err := ListMonitors(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
}

func TestListEnabledMonitors(t *testing.T) {
	db := testDB(t)

	_ = CreateMonitor(db, &Monitor{Name: "Enabled", Platform: "netease", ChartID: "1", Interval: 6, Enabled: true})
	disabled := &Monitor{Name: "Disabled", Platform: "qq", ChartID: "2", Interval: 12, Enabled: true}
	_ = CreateMonitor(db, disabled)
	disabled.Enabled = false
	_ = UpdateMonitor(db, disabled)

	monitors, err := ListEnabledMonitors(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(monitors) != 1 {
		t.Fatalf("expected 1 enabled monitor, got %d", len(monitors))
	}
	if monitors[0].Name != "Enabled" {
		t.Errorf("Name: got %q", monitors[0].Name)
	}
}

// --- MonitorRun ---

func TestMonitorRun_CreateAndFinish(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Run Test", Platform: "netease", ChartID: "1", Interval: 6}
	_ = CreateMonitor(db, m)

	run := &MonitorRun{
		MonitorID: m.ID,
		StartedAt: time.Now(),
		Status:    "running",
	}
	if err := CreateMonitorRun(db, run); err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 {
		t.Fatal("expected non-zero run ID")
	}

	// Finish it.
	now := time.Now()
	run.FinishedAt = &now
	run.TotalFetched = 20
	run.NewQueued = 5
	run.Skipped = 15
	run.Status = "done"
	if err := FinishMonitorRun(db, run); err != nil {
		t.Fatal(err)
	}

	runs, _ := ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != "done" {
		t.Errorf("Status: got %q", runs[0].Status)
	}
	if runs[0].NewQueued != 5 {
		t.Errorf("NewQueued: got %d", runs[0].NewQueued)
	}
}

func TestListMonitorRuns_Order(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Order Test", Platform: "qq", ChartID: "1", Interval: 6}
	_ = CreateMonitor(db, m)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_ = CreateMonitorRun(db, &MonitorRun{
			MonitorID: m.ID,
			StartedAt: base.Add(time.Duration(i) * time.Hour),
			Status:    "done",
		})
	}

	runs, _ := ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	// Should be newest first.
	if runs[0].StartedAt.Before(runs[1].StartedAt) {
		t.Error("runs should be ordered newest first")
	}
}

// --- PruneMonitorRuns ---

func TestPruneMonitorRuns(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Prune Test", Platform: "kugou", ChartID: "6666", Interval: 6}
	_ = CreateMonitor(db, m)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		_ = CreateMonitorRun(db, &MonitorRun{
			MonitorID: m.ID,
			StartedAt: base.Add(time.Duration(i) * time.Hour),
			Status:    "done",
		})
	}

	if err := PruneMonitorRuns(db, m.ID, 3); err != nil {
		t.Fatal(err)
	}

	runs, _ := ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 3 {
		t.Errorf("expected 3 runs after prune, got %d", len(runs))
	}
}

func TestPruneMonitorRuns_NoPruneWhenUnderLimit(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "No Prune", Platform: "netease", ChartID: "1", Interval: 6}
	_ = CreateMonitor(db, m)

	_ = CreateMonitorRun(db, &MonitorRun{
		MonitorID: m.ID,
		StartedAt: time.Now(),
		Status:    "done",
	})

	if err := PruneMonitorRuns(db, m.ID, 100); err != nil {
		t.Fatal(err)
	}

	runs, _ := ListMonitorRuns(db, m.ID, 50)
	if len(runs) != 1 {
		t.Errorf("expected 1 run (no pruning), got %d", len(runs))
	}
}

// --- Dedup: FilterDownloaded ---

func TestFilterDownloaded(t *testing.T) {
	db := testDB(t)

	now := time.Now()
	// Song 1: done, Song 2: failed, Song 3: not in DB.
	_ = SaveTask(db, &download.Task{
		ID: "t-d1", Source: "netease", Song: model.Song{ID: "song1", Name: "A"},
		Status: download.StatusDone, CreatedAt: now,
	})
	_ = SaveTask(db, &download.Task{
		ID: "t-d2", Source: "netease", Song: model.Song{ID: "song2", Name: "B"},
		Status: download.StatusFailed, CreatedAt: now,
	})

	downloaded := FilterDownloaded(db, "netease", []string{"song1", "song2", "song3"})

	if _, ok := downloaded["song1"]; !ok {
		t.Error("song1 (done) should be in downloaded set")
	}
	if _, ok := downloaded["song2"]; ok {
		t.Error("song2 (failed) should NOT be in downloaded set")
	}
	if _, ok := downloaded["song3"]; ok {
		t.Error("song3 (not in DB) should NOT be in downloaded set")
	}
}

func TestFilterDownloaded_Empty(t *testing.T) {
	db := testDB(t)

	result := FilterDownloaded(db, "netease", nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}

	result = FilterDownloaded(db, "netease", []string{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestFilterDownloaded_CrossPlatform(t *testing.T) {
	db := testDB(t)

	now := time.Now()
	// Same song ID on different platforms.
	_ = SaveTask(db, &download.Task{
		ID: "t-x1", Source: "netease", Song: model.Song{ID: "shared-id", Name: "A"},
		Status: download.StatusDone, CreatedAt: now,
	})

	// Query for qq platform — should NOT find the netease record.
	downloaded := FilterDownloaded(db, "qq", []string{"shared-id"})
	if _, ok := downloaded["shared-id"]; ok {
		t.Error("dedup should be platform-scoped; netease record should not match qq query")
	}
}

func TestIsSongDownloaded(t *testing.T) {
	db := testDB(t)

	now := time.Now()
	_ = SaveTask(db, &download.Task{
		ID: "t-isd1", Source: "netease", Song: model.Song{ID: "yes", Name: "A"},
		Status: download.StatusDone, CreatedAt: now,
	})
	_ = SaveTask(db, &download.Task{
		ID: "t-isd2", Source: "netease", Song: model.Song{ID: "no", Name: "B"},
		Status: download.StatusFailed, CreatedAt: now,
	})

	if !IsSongDownloaded(db, "netease", "yes") {
		t.Error("done song should be marked as downloaded")
	}
	if IsSongDownloaded(db, "netease", "no") {
		t.Error("failed song should NOT be marked as downloaded")
	}
	if IsSongDownloaded(db, "netease", "missing") {
		t.Error("non-existent song should NOT be marked as downloaded")
	}
}

// --- UpdateMonitorSchedule ---

func TestUpdateMonitorSchedule(t *testing.T) {
	db := testDB(t)

	m := &Monitor{Name: "Schedule Test", Platform: "netease", ChartID: "1", Interval: 6}
	_ = CreateMonitor(db, m)

	before := time.Now()
	if err := UpdateMonitorSchedule(db, m); err != nil {
		t.Fatal(err)
	}

	got, _ := GetMonitor(db, m.ID)
	if got.LastRunAt == nil {
		t.Fatal("LastRunAt should be set")
	}
	if got.LastRunAt.Before(before) {
		t.Error("LastRunAt should be >= now")
	}
	if got.NextRunAt.Before(got.LastRunAt.Add(5 * time.Hour)) {
		t.Error("NextRunAt should be ~6 hours after LastRunAt")
	}
}
