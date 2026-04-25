package store

import (
	"testing"
	"time"

	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/model"
	"gorm.io/gorm"
)

// testDB creates an in-memory SQLite database for testing.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Use in-memory SQLite for fast, isolated tests.
	db, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return db
}

// --- SaveTask + ListAllTasks round-trip ---

func TestSaveTask_And_ListAllTasks(t *testing.T) {
	db := testDB(t)

	task := &download.Task{
		ID:     "t-aabbccdd11223344",
		Source: "netease",
		Song: model.Song{
			ID:     "12345",
			Name:   "Test Song",
			Artist: "Test Artist",
			Album:  "Test Album",
			Ext:    "mp3",
			Extra:  map[string]string{"quality": "high"},
		},
		Status:    download.StatusDone,
		FilePath:  "/music/Test Artist/Test Album/Test Song.mp3",
		CreatedAt: time.Now().Add(-time.Hour),
	}

	if err := SaveTask(db, task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	tasks, err := ListAllTasks(db)
	if err != nil {
		t.Fatalf("ListAllTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	got := tasks[0]
	if got.ID != "t-aabbccdd11223344" {
		t.Errorf("ID: got %q", got.ID)
	}
	if got.Source != "netease" {
		t.Errorf("Source: got %q", got.Source)
	}
	if got.Song.Name != "Test Song" {
		t.Errorf("Song.Name: got %q", got.Song.Name)
	}
	if got.Song.Artist != "Test Artist" {
		t.Errorf("Song.Artist: got %q", got.Song.Artist)
	}
	if got.Song.Extra["quality"] != "high" {
		t.Errorf("Song.Extra[quality]: got %q", got.Song.Extra["quality"])
	}
	if got.Status != download.StatusDone {
		t.Errorf("Status: got %q", got.Status)
	}
	if got.FilePath != "/music/Test Artist/Test Album/Test Song.mp3" {
		t.Errorf("FilePath: got %q", got.FilePath)
	}
}

func TestSaveTask_Upsert(t *testing.T) {
	db := testDB(t)

	task := &download.Task{
		ID:        "t-upsert001",
		Source:    "qq",
		Song:      model.Song{ID: "99", Name: "Song"},
		Status:    download.StatusPending,
		CreatedAt: time.Now(),
	}
	if err := SaveTask(db, task); err != nil {
		t.Fatal(err)
	}

	// Update status.
	task.Status = download.StatusDone
	task.FilePath = "/music/done.mp3"
	if err := SaveTask(db, task); err != nil {
		t.Fatal(err)
	}

	tasks, _ := ListAllTasks(db)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after upsert, got %d", len(tasks))
	}
	if tasks[0].Status != download.StatusDone {
		t.Fatalf("expected done after upsert, got %s", tasks[0].Status)
	}
	if tasks[0].FilePath != "/music/done.mp3" {
		t.Fatalf("FilePath not updated: %q", tasks[0].FilePath)
	}
}

func TestSaveTask_CreatedAt_ZeroValueDefense(t *testing.T) {
	db := testDB(t)

	task := &download.Task{
		ID:     "t-zero-created",
		Source: "test",
		Song:   model.Song{ID: "1", Name: "Zero"},
		Status: download.StatusPending,
		// CreatedAt intentionally zero.
	}
	if err := SaveTask(db, task); err != nil {
		t.Fatal(err)
	}

	tasks, _ := ListAllTasks(db)
	if len(tasks) != 1 {
		t.Fatal("expected 1 task")
	}
	if tasks[0].CreatedAt.IsZero() {
		t.Fatal("CreatedAt should not be zero — SaveTask should defend against it")
	}
}

// --- MarkRunningAsFailed ---

func TestMarkRunningAsFailed(t *testing.T) {
	db := testDB(t)

	now := time.Now()
	for _, task := range []*download.Task{
		{ID: "t-running", Source: "test", Song: model.Song{ID: "1", Name: "R"}, Status: download.StatusRunning, CreatedAt: now},
		{ID: "t-pending", Source: "test", Song: model.Song{ID: "2", Name: "P"}, Status: download.StatusPending, CreatedAt: now},
		{ID: "t-done", Source: "test", Song: model.Song{ID: "3", Name: "D"}, Status: download.StatusDone, CreatedAt: now},
		{ID: "t-failed", Source: "test", Song: model.Song{ID: "4", Name: "F"}, Status: download.StatusFailed, CreatedAt: now},
	} {
		if err := SaveTask(db, task); err != nil {
			t.Fatal(err)
		}
	}

	if err := MarkRunningAsFailed(db); err != nil {
		t.Fatalf("MarkRunningAsFailed: %v", err)
	}

	tasks, _ := ListAllTasks(db)
	statusMap := make(map[string]download.TaskStatus)
	errorMap := make(map[string]string)
	for _, task := range tasks {
		statusMap[task.ID] = task.Status
		errorMap[task.ID] = task.Error
	}

	if statusMap["t-running"] != download.StatusFailed {
		t.Errorf("running task should be failed, got %s", statusMap["t-running"])
	}
	if errorMap["t-running"] == "" {
		t.Error("running task should have error message")
	}
	if statusMap["t-pending"] != download.StatusFailed {
		t.Errorf("pending task should be failed, got %s", statusMap["t-pending"])
	}
	if statusMap["t-done"] != download.StatusDone {
		t.Errorf("done task should stay done, got %s", statusMap["t-done"])
	}
	if statusMap["t-failed"] != download.StatusFailed {
		t.Errorf("failed task should stay failed, got %s", statusMap["t-failed"])
	}
}

// --- ListAllTasks ordering ---

func TestListAllTasks_OrderByCreatedAt(t *testing.T) {
	db := testDB(t)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Insert in reverse order to verify ORDER BY works.
	for i := 3; i >= 1; i-- {
		task := &download.Task{
			ID:        "t-order-" + string(rune('a'+i-1)),
			Source:    "test",
			Song:      model.Song{ID: string(rune('0' + i)), Name: "Song"},
			Status:    download.StatusDone,
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := SaveTask(db, task); err != nil {
			t.Fatal(err)
		}
	}

	tasks, _ := ListAllTasks(db)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	// Should be ordered by created_at ASC.
	if tasks[0].ID != "t-order-a" || tasks[1].ID != "t-order-b" || tasks[2].ID != "t-order-c" {
		t.Fatalf("wrong order: %s, %s, %s", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

// --- CreateBatch + GetBatch ---

func TestCreateBatch_And_GetBatch(t *testing.T) {
	db := testDB(t)

	if err := CreateBatch(db, "b-test001", "netease", "My Playlist", 10); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	batch, err := GetBatch(db, "b-test001")
	if err != nil {
		t.Fatalf("GetBatch: %v", err)
	}
	if batch.ID != "b-test001" {
		t.Errorf("ID: got %q", batch.ID)
	}
	if batch.Source != "netease" {
		t.Errorf("Source: got %q", batch.Source)
	}
	if batch.Name != "My Playlist" {
		t.Errorf("Name: got %q", batch.Name)
	}
	if batch.Total != 10 {
		t.Errorf("Total: got %d", batch.Total)
	}
}

func TestGetBatch_NotFound(t *testing.T) {
	db := testDB(t)
	_, err := GetBatch(db, "b-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent batch")
	}
}

// --- ListBatchNames ---

func TestListBatchNames(t *testing.T) {
	db := testDB(t)

	_ = CreateBatch(db, "b-001", "qq", "Playlist A", 5)
	_ = CreateBatch(db, "b-002", "netease", "Playlist B", 3)

	names, err := ListBatchNames(db)
	if err != nil {
		t.Fatalf("ListBatchNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names["b-001"] != "Playlist A" {
		t.Errorf("b-001: got %q", names["b-001"])
	}
	if names["b-002"] != "Playlist B" {
		t.Errorf("b-002: got %q", names["b-002"])
	}
}

// --- ListBatchesWithStats ---

func TestListBatchesWithStats(t *testing.T) {
	db := testDB(t)

	// Create 2 batches.
	_ = CreateBatch(db, "b-stat1", "qq", "Batch One", 3)
	_ = CreateBatch(db, "b-stat2", "netease", "Batch Two", 2)

	now := time.Now()
	// Batch One: 2 done, 1 failed.
	for _, task := range []*download.Task{
		{ID: "t-s1a", Source: "qq", BatchID: "b-stat1", Song: model.Song{ID: "1", Name: "A"}, Status: download.StatusDone, CreatedAt: now},
		{ID: "t-s1b", Source: "qq", BatchID: "b-stat1", Song: model.Song{ID: "2", Name: "B"}, Status: download.StatusDone, CreatedAt: now},
		{ID: "t-s1c", Source: "qq", BatchID: "b-stat1", Song: model.Song{ID: "3", Name: "C"}, Status: download.StatusFailed, CreatedAt: now},
	} {
		_ = SaveTask(db, task)
	}
	// Batch Two: 1 done, 1 pending.
	for _, task := range []*download.Task{
		{ID: "t-s2a", Source: "netease", BatchID: "b-stat2", Song: model.Song{ID: "4", Name: "D"}, Status: download.StatusDone, CreatedAt: now},
		{ID: "t-s2b", Source: "netease", BatchID: "b-stat2", Song: model.Song{ID: "5", Name: "E"}, Status: download.StatusPending, CreatedAt: now},
	} {
		_ = SaveTask(db, task)
	}

	batches, err := ListBatchesWithStats(db)
	if err != nil {
		t.Fatalf("ListBatchesWithStats: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}

	// Find each batch (order is DESC by created_at, both created at same time so either order is ok).
	var b1, b2 *BatchWithStats
	for i := range batches {
		switch batches[i].ID {
		case "b-stat1":
			b1 = &batches[i]
		case "b-stat2":
			b2 = &batches[i]
		}
	}

	if b1 == nil || b2 == nil {
		t.Fatal("missing batches")
	}

	if b1.Name != "Batch One" {
		t.Errorf("b1 Name: %q", b1.Name)
	}
	if b1.Total != 3 || b1.Done != 2 || b1.Failed != 1 {
		t.Errorf("b1 stats: total=%d done=%d failed=%d", b1.Total, b1.Done, b1.Failed)
	}

	if b2.Name != "Batch Two" {
		t.Errorf("b2 Name: %q", b2.Name)
	}
	if b2.Total != 2 || b2.Done != 1 || b2.Pending != 1 {
		t.Errorf("b2 stats: total=%d done=%d pending=%d", b2.Total, b2.Done, b2.Pending)
	}
}

// --- Restart simulation ---

func TestRestartRecovery_FullFlow(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	// Simulate: before restart, we had a running task and a done task.
	_ = CreateBatch(db, "b-restart", "qq", "Restart Playlist", 2)
	_ = SaveTask(db, &download.Task{
		ID: "t-r1", Source: "qq", BatchID: "b-restart",
		Song: model.Song{ID: "1", Name: "Running Song"}, Status: download.StatusRunning, CreatedAt: now,
	})
	_ = SaveTask(db, &download.Task{
		ID: "t-r2", Source: "qq", BatchID: "b-restart",
		Song: model.Song{ID: "2", Name: "Done Song"}, Status: download.StatusDone,
		FilePath: "/music/done.mp3", CreatedAt: now,
	})

	// Simulate restart: mark running as failed.
	if err := MarkRunningAsFailed(db); err != nil {
		t.Fatal(err)
	}

	// Reload.
	tasks, _ := ListAllTasks(db)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	statusMap := make(map[string]download.TaskStatus)
	for _, task := range tasks {
		statusMap[task.ID] = task.Status
	}
	if statusMap["t-r1"] != download.StatusFailed {
		t.Errorf("running task should be failed after restart, got %s", statusMap["t-r1"])
	}
	if statusMap["t-r2"] != download.StatusDone {
		t.Errorf("done task should stay done, got %s", statusMap["t-r2"])
	}

	// Verify batch stats from DB.
	batches, _ := ListBatchesWithStats(db)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	b := batches[0]
	if b.Name != "Restart Playlist" {
		t.Errorf("batch name: %q", b.Name)
	}
	if b.Done != 1 || b.Failed != 1 {
		t.Errorf("batch stats: done=%d failed=%d (expected 1,1)", b.Done, b.Failed)
	}

	// Verify batch names can be loaded for Manager.
	names, _ := ListBatchNames(db)
	if names["b-restart"] != "Restart Playlist" {
		t.Errorf("batch name: %q", names["b-restart"])
	}
}
