package monitor

import (
	"log/slog"
	"sync"
	"time"

	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/internal/store"
	"github.com/guohuiyuan/music-lib/model"
	"gorm.io/gorm"
)

// ChartProvider supplies chart songs for a given platform.
type ChartProvider struct {
	GetChartSongs  func(chartID string, limit int) ([]model.Song, error)
	GetDownloadURL func(*model.Song) (string, error)
	GetLyrics      func(*model.Song) (string, error)
}

// Scheduler polls the database for due monitors and executes them.
type Scheduler struct {
	db        *gorm.DB
	dlMgr     *download.Manager
	providers map[string]ChartProvider
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewScheduler creates a new chart monitor scheduler.
func NewScheduler(db *gorm.DB, dlMgr *download.Manager, providers map[string]ChartProvider) *Scheduler {
	return &Scheduler{
		db:        db,
		dlMgr:     dlMgr,
		providers: providers,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the polling loop. It checks for due monitors every minute.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.loop()
	slog.Info("monitor.scheduler.started")
}

// Stop signals the scheduler to stop and waits for it to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	slog.Info("monitor.scheduler.stopped")
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run once immediately at startup to catch any overdue monitors.
	s.tick()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	monitors, err := store.ListEnabledMonitors(s.db)
	if err != nil {
		slog.Warn("monitor.scheduler.list_error", "error", err)
		return
	}

	now := time.Now()
	for i := range monitors {
		m := &monitors[i]
		if m.NextRunAt.After(now) {
			continue // not due yet
		}
		s.execute(m)
	}
}

// Execute runs a single monitor. Called by the scheduler loop or manual trigger.
func (s *Scheduler) Execute(monitorID uint) {
	m, err := store.GetMonitor(s.db, monitorID)
	if err != nil {
		slog.Warn("monitor.execute.not_found", "id", monitorID, "error", err)
		return
	}
	s.execute(m)
}

func (s *Scheduler) execute(m *store.Monitor) {
	provider, ok := s.providers[m.Platform]
	if !ok {
		slog.Warn("monitor.execute.no_provider", "platform", m.Platform, "monitor_id", m.ID)
		return
	}

	slog.Info("monitor.execute.start", "monitor_id", m.ID, "name", m.Name, "chart", m.ChartID)

	// Create run record.
	run := &store.MonitorRun{
		MonitorID: m.ID,
		StartedAt: time.Now(),
		Status:    "running",
	}
	if err := store.CreateMonitorRun(s.db, run); err != nil {
		slog.Warn("monitor.execute.create_run", "error", err)
		return
	}

	// Fetch chart songs.
	songs, err := provider.GetChartSongs(m.ChartID, m.TopN)
	if err != nil {
		s.finishRun(run, 0, 0, 0, "failed", err.Error())
		store.UpdateMonitorSchedule(s.db, m)
		slog.Warn("monitor.execute.fetch_error", "monitor_id", m.ID, "error", err)
		return
	}

	run.TotalFetched = len(songs)

	// Dedup: find which songs are already downloaded.
	songIDs := make([]string, len(songs))
	for i, song := range songs {
		songIDs[i] = song.ID
	}
	downloaded := store.FilterDownloaded(s.db, m.Platform, songIDs)

	// Enqueue new songs.
	var newSongs []model.Song
	for _, song := range songs {
		if _, exists := downloaded[song.ID]; exists {
			continue
		}
		newSongs = append(newSongs, song)
	}

	run.NewQueued = len(newSongs)
	run.Skipped = run.TotalFetched - run.NewQueued

	if len(newSongs) > 0 && s.dlMgr != nil {
		batchName := m.Name + " - " + time.Now().Format("2006-01-02")
		s.dlMgr.EnqueueBatch(
			newSongs,
			batchName,
			m.Platform,
			provider.GetDownloadURL,
			provider.GetLyrics,
		)
	}

	s.finishRun(run, run.TotalFetched, run.NewQueued, run.Skipped, "done", "")

	// Update schedule.
	if err := store.UpdateMonitorSchedule(s.db, m); err != nil {
		slog.Warn("monitor.execute.update_schedule", "error", err)
	}

	// Prune old runs (keep last 100).
	store.PruneMonitorRuns(s.db, m.ID, 100)

	slog.Info("monitor.execute.done",
		"monitor_id", m.ID,
		"fetched", run.TotalFetched,
		"new", run.NewQueued,
		"skipped", run.Skipped,
	)
}

func (s *Scheduler) finishRun(run *store.MonitorRun, fetched, queued, skipped int, status, errMsg string) {
	now := time.Now()
	run.FinishedAt = &now
	run.TotalFetched = fetched
	run.NewQueued = queued
	run.Skipped = skipped
	run.Status = status
	run.Error = errMsg
	if err := store.FinishMonitorRun(s.db, run); err != nil {
		slog.Warn("monitor.execute.finish_run", "error", err)
	}
}
