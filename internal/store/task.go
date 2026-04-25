package store

import (
	"time"

	"github.com/guohuiyuan/music-lib/download"
	"github.com/guohuiyuan/music-lib/model"
	"gorm.io/gorm"
)

// TaskRecord is the GORM model for the download_tasks table.
type TaskRecord struct {
	ID             string     `gorm:"primaryKey"`
	Source         string     `gorm:"not null;index:idx_source_song,composite:source"`
	FallbackSource string
	BatchID        string     `gorm:"index"`
	SongID         string     `gorm:"index:idx_source_song,composite:song"`
	Title          string     `gorm:"not null"`
	Artist         string
	Album          string
	Ext            string
	Quality        string
	FilePath       string
	Status         string     `gorm:"not null;index"`
	Error          string
	RetryCount     int        `gorm:"default:0"`
	Skipped        bool
	Progress       int64
	TotalSize      int64
	ScrapeStatus   string
	ScrapeError    string
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
	CompletedAt    *time.Time
	ScrapedAt      *time.Time
}

// TableName overrides the default table name.
func (TaskRecord) TableName() string { return "download_tasks" }

// SaveTask upserts a download.Task into the database.
func SaveTask(db *gorm.DB, t *download.Task) error {
	quality := ""
	if t.Song.Extra != nil {
		quality = t.Song.Extra["quality"]
	}
	now := time.Now()
	createdAt := t.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	r := TaskRecord{
		ID:             t.ID,
		Source:         t.Source,
		FallbackSource: t.FallbackSource,
		BatchID:        t.BatchID,
		SongID:         t.Song.ID,
		Title:          t.Song.Name,
		Artist:         t.Song.Artist,
		Album:          t.Song.Album,
		Ext:            t.Song.Ext,
		Quality:        quality,
		FilePath:       t.FilePath,
		Status:         string(t.Status),
		Error:          t.Error,
		RetryCount:     t.RetryCount,
		Skipped:        t.Skipped,
		Progress:       t.Progress,
		TotalSize:      t.TotalSize,
		ScrapeStatus:   t.ScrapeStatus,
		ScrapeError:    t.ScrapeError,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
		CompletedAt:    t.CompletedAt,
		ScrapedAt:      t.ScrapedAt,
	}
	return db.Save(&r).Error
}

// ListAllTasks reads all TaskRecords from the database and converts them to
// []*download.Task. The returned tasks contain no live URL/Cover fields
// (those are runtime-only), but are suitable for restoring history state.
func ListAllTasks(db *gorm.DB) ([]*download.Task, error) {
	var records []TaskRecord
	if err := db.Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	tasks := make([]*download.Task, 0, len(records))
	for _, r := range records {
		extra := map[string]string{}
		if r.Quality != "" {
			extra["quality"] = r.Quality
		}
		song := model.Song{
			ID:     r.SongID,
			Name:   r.Title,
			Artist: r.Artist,
			Album:  r.Album,
			Ext:    r.Ext,
			Extra:  extra,
		}
		t := &download.Task{
			ID:             r.ID,
			Source:         r.Source,
			FallbackSource: r.FallbackSource,
			BatchID:        r.BatchID,
			FilePath:       r.FilePath,
			Song:           song,
			Status:         download.TaskStatus(r.Status),
			Error:          r.Error,
			RetryCount:     r.RetryCount,
			Skipped:        r.Skipped,
			Progress:       r.Progress,
			TotalSize:      r.TotalSize,
			ScrapeStatus:   r.ScrapeStatus,
			ScrapeError:    r.ScrapeError,
			CreatedAt:      r.CreatedAt,
			CompletedAt:    r.CompletedAt,
			ScrapedAt:      r.ScrapedAt,
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// MarkRunningAsFailed marks all running and pending tasks as failed.
// Called at startup to handle tasks that were interrupted by a previous restart.
func MarkRunningAsFailed(db *gorm.DB) error {
	return db.Model(&TaskRecord{}).
		Where("status IN ?", []string{"running", "pending"}).
		Updates(map[string]any{
			"status":     "failed",
			"error":      "interrupted by server restart (was running)",
			"updated_at": time.Now(),
		}).Error
}
