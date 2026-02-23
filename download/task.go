package download

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guohuiyuan/music-lib/model"
)

// TaskStatus represents the current state of a download task.
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// Task represents a single song download job.
type Task struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	BatchID     string     `json:"batch_id,omitempty"`
	Error       string     `json:"error,omitempty"`
	FilePath    string     `json:"file_path,omitempty"`
	Song        model.Song `json:"song"`
	Status      TaskStatus `json:"status"`
	Skipped     bool       `json:"skipped"`
	Progress    int64      `json:"progress"`
	TotalSize   int64      `json:"total_size"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// BatchInfo summarizes the state of a batch download.
type BatchInfo struct {
	ID           string `json:"id"`
	PlaylistName string `json:"playlist_name"`
	Total        int    `json:"total"`
	Completed    int    `json:"completed"`
	Failed       int    `json:"failed"`
	Running      int    `json:"running"`
	Pending      int    `json:"pending"`
}

// Manager coordinates download tasks with bounded concurrency.
type Manager struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	order    []string
	counter  int64
	sem      chan struct{}
	musicDir string
}

// NewManager creates a Manager. If concurrency <= 0 it defaults to 3.
func NewManager(musicDir string, concurrency int) *Manager {
	if concurrency <= 0 {
		concurrency = 3
	}
	return &Manager{
		tasks:    make(map[string]*Task),
		sem:      make(chan struct{}, concurrency),
		musicDir: musicDir,
	}
}

// MusicDir returns the configured base directory for NAS downloads.
func (m *Manager) MusicDir() string {
	return m.musicDir
}

// Concurrency returns the max concurrent downloads.
func (m *Manager) Concurrency() int {
	return cap(m.sem)
}

// Enqueue creates a single download task and starts it in a goroutine.
// getURL and getLyrics are called inside the goroutine so URLs are fetched just-in-time.
func (m *Manager) Enqueue(
	song model.Song,
	source string,
	getURL func(*model.Song) (string, error),
	getLyrics func(*model.Song) (string, error),
) string {
	id := fmt.Sprintf("t-%d", atomic.AddInt64(&m.counter, 1))
	task := &Task{
		ID:        id,
		Source:    source,
		Song:      song,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	m.mu.Lock()
	m.tasks[id] = task
	m.order = append(m.order, id)
	m.mu.Unlock()

	go m.runTask(task, getURL, getLyrics)
	return id
}

// EnqueueBatch creates tasks for multiple songs sharing a batch ID.
func (m *Manager) EnqueueBatch(
	songs []model.Song,
	playlistName string,
	source string,
	getURL func(*model.Song) (string, error),
	getLyrics func(*model.Song) (string, error),
) string {
	batchID := fmt.Sprintf("b-%d", atomic.AddInt64(&m.counter, 1))

	for i := range songs {
		id := fmt.Sprintf("t-%d", atomic.AddInt64(&m.counter, 1))
		task := &Task{
			ID:        id,
			Source:    source,
			BatchID:   batchID,
			Song:      songs[i],
			Status:    StatusPending,
			CreatedAt: time.Now(),
		}

		m.mu.Lock()
		m.tasks[id] = task
		m.order = append(m.order, id)
		m.mu.Unlock()

		go m.runTask(task, getURL, getLyrics)
	}

	// Store a synthetic batch entry so ListBatches can find the playlist name.
	m.mu.Lock()
	m.tasks[batchID] = &Task{
		ID:     batchID,
		Source: source,
		Song:   model.Song{Name: playlistName},
		Status: StatusCompleted,
	}
	m.mu.Unlock()

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
		if t, ok := m.tasks[id]; ok {
			result = append(result, t)
		}
	}
	return result
}

// ListBatches aggregates task counts per batch.
func (m *Manager) ListBatches() []BatchInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type acc struct {
		name                                  string
		total, completed, failed, running, pending int
	}
	batches := make(map[string]*acc)
	var batchOrder []string

	for _, id := range m.order {
		t := m.tasks[id]
		if t.BatchID == "" {
			continue
		}
		a, exists := batches[t.BatchID]
		if !exists {
			name := t.BatchID
			if bt, ok := m.tasks[t.BatchID]; ok {
				name = bt.Song.Name
			}
			a = &acc{name: name}
			batches[t.BatchID] = a
			batchOrder = append(batchOrder, t.BatchID)
		}
		a.total++
		switch t.Status {
		case StatusCompleted:
			a.completed++
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
		a := batches[bid]
		result = append(result, BatchInfo{
			ID:           bid,
			PlaylistName: a.name,
			Total:        a.total,
			Completed:    a.completed,
			Failed:       a.failed,
			Running:      a.running,
			Pending:      a.pending,
		})
	}
	return result
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
	m.mu.Unlock()

	// 1. Get download URL (just-in-time).
	audioURL, err := getURL(&task.Song)
	if err != nil {
		m.failTask(task, fmt.Sprintf("get download url: %v", err))
		return
	}

	// 2. Get lyrics (best-effort).
	var lyrics string
	if getLyrics != nil {
		lyrics, err = getLyrics(&task.Song)
		if err != nil {
			log.Printf("[download] lyrics skipped for %s: %v", task.Song.Display(), err)
		}
	}

	// 3. Write song to disk with progress tracking.
	progressFn := func(n int64) {
		m.mu.Lock()
		task.Progress = n
		m.mu.Unlock()
	}
	filePath, skipped, err := WriteSongToDisk(m.musicDir, &task.Song, audioURL, lyrics, progressFn)
	if err != nil {
		m.failTask(task, fmt.Sprintf("write to disk: %v", err))
		return
	}

	// 4. Download cover (best-effort).
	if task.Song.Cover != "" {
		coverDir := buildSongDir(m.musicDir, &task.Song)
		if coverErr := saveCover(coverDir, task.Song.Cover); coverErr != nil {
			log.Printf("[download] cover skipped for %s: %v", task.Song.Display(), coverErr)
		}
	}

	// 5. Mark completed.
	now := time.Now()
	m.mu.Lock()
	task.Status = StatusCompleted
	task.FilePath = filePath
	task.Skipped = skipped
	task.CompletedAt = &now
	m.mu.Unlock()
}

func (m *Manager) failTask(task *Task, msg string) {
	now := time.Now()
	m.mu.Lock()
	task.Status = StatusFailed
	task.Error = msg
	task.CompletedAt = &now
	m.mu.Unlock()
	log.Printf("[download] task %s failed: %s — %s", task.ID, task.Song.Display(), msg)
}
