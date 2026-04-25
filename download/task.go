package download

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"github.com/guohuiyuan/music-lib/scrape"
)

// TaskStatus represents the current state of a download task.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusDone      TaskStatus = "done"
	StatusFailed    TaskStatus = "failed"
	// StatusCompleted is kept as alias so callers that used it still compile.
	StatusCompleted = StatusDone
)

// Task represents a single song download job.
type Task struct {
	ID             string     `json:"id"`
	Source         string     `json:"source"`
	FallbackSource string     `json:"fallback_source,omitempty"`
	BatchID        string     `json:"batch_id,omitempty"`
	Error          string     `json:"error,omitempty"`
	FilePath       string     `json:"file_path,omitempty"`
	Song           model.Song `json:"song"`
	Status         TaskStatus `json:"status"`
	RetryCount     int        `json:"retry_count"`
	Skipped        bool       `json:"skipped"`
	Progress       int64      `json:"progress"`
	TotalSize      int64      `json:"total_size"`
	ScrapeStatus   string     `json:"scrape_status,omitempty"` // pending/done/failed/skipped
	ScrapeError    string     `json:"scrape_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ScrapedAt      *time.Time `json:"scraped_at,omitempty"`

	// Quality tracking fields (added in M0).
	RequestedQuality string `json:"requested_quality,omitempty"` // lossless / high / standard
	ActualQuality    string `json:"actual_quality,omitempty"`    // e.g. "FLAC", "320kbps MP3"
	Upgraded         bool   `json:"upgraded"`                    // true when an existing file was replaced
	PreviousQuality  string `json:"previous_quality,omitempty"`  // quality of the replaced file
}

// UpgradeResult is the response body for POST /api/nas/download/upgrade.
type UpgradeResult struct {
	BatchID string         `json:"batch_id"`
	Queued  int            `json:"queued"`
	Skipped int            `json:"skipped"`
	Errors  []UpgradeError `json:"errors"`
}

// UpgradeError describes a single task that could not be re-queued.
type UpgradeError struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// BatchInfo summarizes the state of a batch download.
type BatchInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Failed  int    `json:"failed"`
	Running int    `json:"running"`
	Pending int    `json:"pending"`
}

// Config holds Manager configuration.
type Config struct {
	MusicDir      string
	Concurrency   int
	MaxRetries    int // default 3
	RetryBackoff  int // backoff base in seconds, default 2
	ScrapeEnabled bool
	ScrapeCover   bool
	ScrapeLyrics  bool
}

// Manager coordinates download tasks with bounded concurrency.
type Manager struct {
	mu           sync.RWMutex
	tasks        map[string]*Task
	order        []string
	batches      map[string]string // batchID -> batchName (separated from tasks)
	sem          chan struct{}
	cfg          Config
	providers    map[string]ProviderFuncs
	onTaskUpdate func(task *Task)
	updateCh     chan Task // serialized write queue for DB persistence
}

// NewManager creates a Manager using the given Config.
// If cfg.Concurrency <= 0 it defaults to 3.
// If cfg.MaxRetries <= 0 it defaults to 3.
// If cfg.RetryBackoff <= 0 it defaults to 2.
func NewManager(cfg Config, providers map[string]ProviderFuncs) *Manager {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 3
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 2
	}
	m := &Manager{
		tasks:    make(map[string]*Task),
		batches:  make(map[string]string),
		sem:      make(chan struct{}, cfg.Concurrency),
		cfg:      cfg,
		providers: providers,
		updateCh: make(chan Task, 256),
	}
	go m.drainUpdates()
	return m
}

// SetOnTaskUpdate registers a callback called whenever a task's state changes.
func (m *Manager) SetOnTaskUpdate(fn func(*Task)) {
	m.mu.Lock()
	m.onTaskUpdate = fn
	m.mu.Unlock()
}

// LoadTasks populates the in-memory map from a persisted task slice.
// Called once at startup after MarkRunningAsFailed.
func (m *Manager) LoadTasks(tasks []*Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range tasks {
		if _, exists := m.tasks[t.ID]; !exists {
			m.tasks[t.ID] = t
			m.order = append(m.order, t.ID)
		}
	}
}

// MusicDir returns the configured base directory for NAS downloads.
func (m *Manager) MusicDir() string {
	return m.cfg.MusicDir
}

// Concurrency returns the max concurrent downloads.
func (m *Manager) Concurrency() int {
	return cap(m.sem)
}

// notifyUpdate enqueues a deep-copied snapshot of task for serialized DB write.
// Must be called while holding m.mu (write lock).
func (m *Manager) notifyUpdate(task *Task) {
	if m.onTaskUpdate == nil {
		return
	}
	t := *task
	// Deep copy Song.Extra to avoid concurrent map access.
	if task.Song.Extra != nil {
		t.Song.Extra = make(map[string]string, len(task.Song.Extra))
		for k, v := range task.Song.Extra {
			t.Song.Extra[k] = v
		}
	}
	select {
	case m.updateCh <- t:
	default:
		// Channel full — drop oldest by draining one, then send.
		<-m.updateCh
		m.updateCh <- t
	}
}

// drainUpdates processes the serialized write queue in a single goroutine,
// guaranteeing DB writes happen in the same order as state changes.
func (m *Manager) drainUpdates() {
	for t := range m.updateCh {
		m.mu.RLock()
		fn := m.onTaskUpdate
		m.mu.RUnlock()
		if fn != nil {
			fn(&t)
		}
	}
}

// Enqueue creates a single download task and starts it in a goroutine.
func (m *Manager) Enqueue(
	song model.Song,
	source string,
	getURL func(*model.Song) (string, error),
	getLyrics func(*model.Song) (string, error),
) string {
	id := newID("t")
	requestedQuality := ""
	if song.Extra != nil {
		requestedQuality = song.Extra["quality"]
	}
	task := &Task{
		ID:               id,
		Source:           source,
		Song:             song,
		Status:           StatusPending,
		CreatedAt:        time.Now(),
		RequestedQuality: requestedQuality,
	}

	m.mu.Lock()
	m.tasks[id] = task
	m.order = append(m.order, id)
	m.notifyUpdate(task)
	m.mu.Unlock()

	slog.Info("download.enqueue",
		"task_id", id,
		"title", song.Name,
		"artist", song.Artist,
		"source", source,
	)

	go m.runTask(task, getURL, getLyrics)
	return id
}

// EnqueueBatch creates tasks for multiple songs sharing a batch ID.
// The batchName is stored in the synthetic batch task (for ListBatches).
func (m *Manager) EnqueueBatch(
	songs []model.Song,
	batchName string,
	source string,
	getURL func(*model.Song) (string, error),
	getLyrics func(*model.Song) (string, error),
) string {
	batchID := newID("b")

	m.mu.Lock()
	m.batches[batchID] = batchName
	m.mu.Unlock()

	for i := range songs {
		id := newID("t")
		requestedQuality := ""
		if songs[i].Extra != nil {
			requestedQuality = songs[i].Extra["quality"]
		}
		task := &Task{
			ID:               id,
			Source:           source,
			BatchID:          batchID,
			Song:             songs[i],
			Status:           StatusPending,
			CreatedAt:        time.Now(),
			RequestedQuality: requestedQuality,
		}

		m.mu.Lock()
		m.tasks[id] = task
		m.order = append(m.order, id)
		m.notifyUpdate(task)
		m.mu.Unlock()

		go m.runTask(task, getURL, getLyrics)
	}

	return batchID
}

// GetTask returns a task by ID.
func (m *Manager) GetTask(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return t, ok
}

// ListTasks returns all non-batch tasks in creation order.
func (m *Manager) ListTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		t, ok := m.tasks[id]
		if !ok {
			continue
		}
		// Skip synthetic batch entries (those with IDs starting "b-").
		if strings.HasPrefix(id, "b-") {
			continue
		}
		result = append(result, t)
	}
	return result
}

// ListBatches aggregates task counts per batch from in-memory state.
// After restart, this only reflects tasks loaded via LoadTasks.
// For persistent batch data, use store.ListBatchesWithStats from the handler.
func (m *Manager) ListBatches() []BatchInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type acc struct {
		name                                   string
		total, done, failed, running, pending int
	}
	agg := make(map[string]*acc)
	var batchOrder []string

	for _, id := range m.order {
		t := m.tasks[id]
		if t.BatchID == "" {
			continue
		}
		a, exists := agg[t.BatchID]
		if !exists {
			name := m.batches[t.BatchID]
			if name == "" {
				name = t.BatchID
			}
			a = &acc{name: name}
			agg[t.BatchID] = a
			batchOrder = append(batchOrder, t.BatchID)
		}
		a.total++
		switch t.Status {
		case StatusDone:
			a.done++
		case StatusFailed:
			a.failed++
		case StatusRunning:
			a.running++
		default:
			a.pending++
		}
	}

	result := make([]BatchInfo, 0, len(batchOrder))
	for _, bid := range batchOrder {
		a := agg[bid]
		result = append(result, BatchInfo{
			ID:      bid,
			Name:    a.name,
			Total:   a.total,
			Done:    a.done,
			Failed:  a.failed,
			Running: a.running,
			Pending: a.pending,
		})
	}
	return result
}

// LoadBatchNames populates the in-memory batches map from persisted data.
// Called once at startup after LoadTasks.
func (m *Manager) LoadBatchNames(names map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, name := range names {
		if _, exists := m.batches[id]; !exists {
			m.batches[id] = name
		}
	}
}

// runTask executes a download in a goroutine bounded by the semaphore.
func (m *Manager) runTask(
	task *Task,
	getURL func(*model.Song) (string, error),
	getLyrics func(*model.Song) (string, error),
) {
	// Acquire semaphore slot.
	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	m.mu.Lock()
	task.Status = StatusRunning
	m.notifyUpdate(task)
	m.mu.Unlock()

	// 1. Get download URL with retry on transient errors.
	var audioURL string
	var lastGetURLErr error

	getURLFn := func() error {
		url, err := getURL(&task.Song)
		if err != nil {
			return err
		}
		if url == "" {
			// Empty URL = platform refused — not retryable, treat as a hard error.
			return errEmptyURL
		}
		audioURL = url
		return nil
	}

	var totalAttempts int
	if err := withRetry(m.cfg.MaxRetries, m.cfg.RetryBackoff, getURLFn, func(attempt int, waitMs int64, err error) {
		totalAttempts = attempt
		m.mu.Lock()
		task.RetryCount = attempt
		m.mu.Unlock()
		slog.Warn("download.retry",
			"task_id", task.ID,
			"attempt", attempt,
			"max", m.cfg.MaxRetries,
			"error", err,
			"wait_ms", waitMs,
		)
	}); err != nil {
		lastGetURLErr = err
		// Ensure RetryCount reflects final attempt count.
		finalAttempts := totalAttempts + 1 // last attempt not tracked by onRetry
		if finalAttempts > m.cfg.MaxRetries {
			finalAttempts = m.cfg.MaxRetries
		}
		m.mu.Lock()
		task.RetryCount = finalAttempts
		m.mu.Unlock()
		// Primary source failed after all retries — try fallback.
		fallbackURL, fbSource, fbErr := m.tryFallback(task.Song, task.Source)
		if fbErr != nil {
			m.failTask(task, fmt.Sprintf(
				"primary source %s failed after %d attempts (last: %v); all fallback providers exhausted",
				task.Source, finalAttempts, lastGetURLErr,
			))
			return
		}
		audioURL = fallbackURL
		slog.Warn("download.fallback",
			"task_id", task.ID,
			"from", task.Source,
			"to", fbSource,
			"retry_exhausted", true,
		)
		m.mu.Lock()
		task.FallbackSource = fbSource
		m.notifyUpdate(task)
		m.mu.Unlock()
	}

	// 2. Get lyrics (best-effort).
	var lyrics string
	if getLyrics != nil {
		var err error
		lyrics, err = getLyrics(&task.Song)
		if err != nil {
			slog.Warn("download lyrics skipped", "task_id", task.ID, "song", task.Song.Display(), "error", err)
		}
	}

	// 3. Write song to disk with progress tracking.
	progressFn := func(n int64) {
		m.mu.Lock()
		task.Progress = n
		m.mu.Unlock()
	}
	writeResult, err := WriteSongToDisk(m.cfg.MusicDir, &task.Song, audioURL, lyrics, progressFn)
	if err != nil {
		m.failTask(task, fmt.Sprintf("write to disk: %v", err))
		return
	}

	// 4. Download cover (best-effort) — external cover.jpg for Plex/Navidrome.
	if task.Song.Cover != "" {
		coverDir := buildSongDir(m.cfg.MusicDir, &task.Song)
		if coverErr := saveCover(coverDir, task.Song.Cover); coverErr != nil {
			slog.Warn("download cover skipped", "task_id", task.ID, "song", task.Song.Display(), "error", coverErr)
		}
	}

	// 5. Scrape: embed ID3/Vorbis tags + cover + lyrics into the audio file.
	// Skip scraping when the file was not newly written (skipped or upgraded uses new file).
	if writeResult.Action != ActionSkipped {
		result := scrape.Scrape(scrape.Config{
			Enabled: m.cfg.ScrapeEnabled,
			Cover:   m.cfg.ScrapeCover,
			Lyrics:  m.cfg.ScrapeLyrics,
		}, &task.Song, writeResult.FilePath, lyrics)

		scrapeNow := time.Now()
		m.mu.Lock()
		task.ScrapeStatus = result.Status
		task.ScrapeError = result.Error
		task.ScrapedAt = &scrapeNow
		m.notifyUpdate(task)
		m.mu.Unlock()

		if result.Status == "failed" {
			slog.Warn("scrape.failed", "task_id", task.ID, "song", task.Song.Display(), "error", result.Error)
		} else {
			slog.Info("scrape.done", "task_id", task.ID, "status", result.Status)
		}
	}

	// 6. Mark done.
	now := time.Now()
	actualQuality := task.Song.QualityString()
	var upgraded bool
	var previousQuality string
	if writeResult.Action == ActionUpgraded {
		upgraded = true
		previousQuality = qualityScoreToLabel(writeResult.PreviousExt)
	}

	m.mu.Lock()
	task.Status = StatusDone
	task.FilePath = writeResult.FilePath
	task.Skipped = writeResult.Action == ActionSkipped
	task.ActualQuality = actualQuality
	task.Upgraded = upgraded
	task.PreviousQuality = previousQuality
	task.CompletedAt = &now
	m.notifyUpdate(task)
	m.mu.Unlock()

	slog.Info("download.done",
		"task_id", task.ID,
		"file", writeResult.FilePath,
		"action", writeResult.Action,
		"actual_quality", actualQuality,
	)
	slog.Info("download.quality",
		"task_id", task.ID,
		"requested", task.RequestedQuality,
		"actual_ext", task.Song.Ext,
		"actual_bitrate", task.Song.Bitrate,
		"degraded", task.RequestedQuality == "lossless" && task.Song.Ext != "flac" && task.Song.Ext != "wav",
	)
}

// qualityScoreToLabel returns a human-readable quality string derived only from
// a file extension (used for the previous-file label when bitrate is unknown).
func qualityScoreToLabel(ext string) string {
	switch strings.ToLower(ext) {
	case "flac":
		return "FLAC"
	case "wav":
		return "WAV"
	case "mp3":
		return "MP3"
	case "m4a":
		return "M4A"
	default:
		if ext == "" {
			return ""
		}
		return strings.ToUpper(ext)
	}
}

// EnqueueUpgrade re-queues completed tasks so they can be downloaded again at
// a (potentially higher) quality level. If taskIDs is empty, all tasks with
// status=done and skipped=false are considered.
// quality specifies the target quality tier (lossless/high/standard).
func (m *Manager) EnqueueUpgrade(taskIDs []string, quality string) UpgradeResult {
	if quality == "" {
		quality = "lossless"
	}

	batchID := newID("b")
	result := UpgradeResult{BatchID: batchID, Errors: []UpgradeError{}}

	m.mu.RLock()
	var candidates []*Task
	if len(taskIDs) == 0 {
		// Upgrade all done, non-skipped tasks.
		for _, id := range m.order {
			t := m.tasks[id]
			if t.Status == StatusDone && !t.Skipped {
				candidates = append(candidates, t)
			}
		}
	} else {
		for _, id := range taskIDs {
			t, ok := m.tasks[id]
			if !ok {
				result.Skipped++
				result.Errors = append(result.Errors, UpgradeError{TaskID: id, Reason: "task not found"})
				continue
			}
			candidates = append(candidates, t)
		}
	}
	m.mu.RUnlock()

	for _, t := range candidates {
		if t.Status != StatusDone {
			result.Skipped++
			result.Errors = append(result.Errors, UpgradeError{
				TaskID: t.ID,
				Reason: fmt.Sprintf("task status is %s, not done", t.Status),
			})
			continue
		}
		if t.Source == "" {
			result.Skipped++
			result.Errors = append(result.Errors, UpgradeError{TaskID: t.ID, Reason: "task has no source"})
			continue
		}

		pf, ok := m.providers[t.Source]
		if !ok || pf.GetDownloadURL == nil {
			result.Skipped++
			result.Errors = append(result.Errors, UpgradeError{
				TaskID: t.ID,
				Reason: fmt.Sprintf("provider %q not available", t.Source),
			})
			continue
		}

		// Deep-copy the song and set the target quality.
		songCopy := t.Song
		if songCopy.Extra == nil {
			songCopy.Extra = map[string]string{}
		} else {
			extra := make(map[string]string, len(songCopy.Extra))
			for k, v := range songCopy.Extra {
				extra[k] = v
			}
			songCopy.Extra = extra
		}
		songCopy.Extra["quality"] = quality

		newID := m.Enqueue(songCopy, t.Source, pf.GetDownloadURL, pf.GetLyrics)

		// Tag the new task with requested quality and the upgrade batch ID.
		m.mu.Lock()
		if newTask, exists := m.tasks[newID]; exists {
			newTask.BatchID = batchID
			newTask.RequestedQuality = quality
		}
		m.mu.Unlock()

		result.Queued++
	}

	// Register the upgrade batch name.
	if result.Queued > 0 {
		m.mu.Lock()
		m.batches[batchID] = fmt.Sprintf("音质升级 (%s)", quality)
		m.mu.Unlock()
	}

	return result
}

func (m *Manager) failTask(task *Task, msg string) {
	now := time.Now()
	m.mu.Lock()
	task.Status = StatusFailed
	task.Error = msg
	task.CompletedAt = &now
	m.notifyUpdate(task)
	m.mu.Unlock()
	slog.Error("download.failed",
		"task_id", task.ID,
		"song", task.Song.Display(),
		"error", msg,
	)
}

// --- package-level helpers ---

// errEmptyURL is returned when a provider returns an empty URL (non-retryable).
var errEmptyURL = errors.New("empty download URL (platform refused or no copyright)")

// newID generates a random ID with the given prefix, e.g. "t-{16hex}" or "b-{16hex}".
func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failures are extremely rare; fall back to time-based.
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}

// isRetryable returns true when the error should trigger an automatic retry.
// Retryable: network timeouts, HTTP 5xx, HTTP 429, generic net.OpError.
// Not retryable: HTTP 404, HTTP 403, empty URL, other HTTP 4xx.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errEmptyURL) {
		return false
	}

	// Structured HTTP error from downloadFile / writer.go.
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == 429:
			return true // rate limited
		case httpErr.StatusCode >= 500:
			return true // server error
		default:
			return false // 4xx (404, 403, etc.) — not retryable
		}
	}

	// net.Error with Timeout flag.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Generic network errors (connection refused, reset, etc.) are retryable.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return false
}

// withRetry calls fn up to maxRetries times with exponential backoff.
// Backoff schedule: backoffBase^(attempt-1) seconds (1s, 2s, 4s for base=2).
// onRetry is called before each sleep with the current attempt number and wait duration.
// Returns nil on first success, or the last error after all attempts.
func withRetry(maxRetries, backoffBase int, fn func() error, onRetry func(attempt int, waitMs int64, err error)) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) {
			return lastErr
		}
		if attempt == maxRetries {
			break
		}
		// Exponential backoff: backoffBase^(attempt-1) seconds, capped at 60s.
		waitSec := math.Pow(float64(backoffBase), float64(attempt-1))
		if waitSec > 60 {
			waitSec = 60
		}
		waitDur := time.Duration(waitSec * float64(time.Second))
		if onRetry != nil {
			onRetry(attempt, waitDur.Milliseconds(), lastErr)
		}
		time.Sleep(waitDur)
	}
	return lastErr
}

