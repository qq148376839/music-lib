package store

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Monitor is the GORM model for chart monitoring rules.
type Monitor struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"not null" json:"name"`
	Platform  string     `gorm:"not null;index" json:"platform"`
	ChartID   string     `gorm:"not null" json:"chart_id"`
	TopN      int        `gorm:"not null;default:20" json:"top_n"`
	Interval  int        `gorm:"not null;default:12" json:"interval"` // hours: 6/12/24
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastRunAt *time.Time `json:"last_run_at"`
	NextRunAt time.Time  `gorm:"not null" json:"next_run_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// MonitorRun is the GORM model for monitoring execution history.
type MonitorRun struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	MonitorID    uint       `gorm:"not null;index" json:"monitor_id"`
	StartedAt    time.Time  `gorm:"not null" json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	TotalFetched int        `json:"total_fetched"`
	NewQueued    int        `json:"new_queued"`
	Skipped      int        `json:"skipped"`
	Status       string     `gorm:"not null;default:running" json:"status"` // running/done/failed
	Error        string     `json:"error,omitempty"`
}

// CreateMonitor inserts a new monitor rule.
func CreateMonitor(db *gorm.DB, m *Monitor) error {
	if m.TopN <= 0 {
		m.TopN = 20
	}
	if m.TopN > 100 {
		m.TopN = 100
	}
	if m.Interval != 6 && m.Interval != 12 && m.Interval != 24 {
		m.Interval = 12
	}
	m.NextRunAt = time.Now().Add(time.Duration(m.Interval) * time.Hour)
	return db.Create(m).Error
}

// UpdateMonitor updates an existing monitor rule.
func UpdateMonitor(db *gorm.DB, m *Monitor) error {
	if m.TopN <= 0 {
		m.TopN = 20
	}
	if m.TopN > 100 {
		m.TopN = 100
	}
	if m.Interval != 6 && m.Interval != 12 && m.Interval != 24 {
		m.Interval = 12
	}
	return db.Save(m).Error
}

// DeleteMonitor removes a monitor rule and its run history.
func DeleteMonitor(db *gorm.DB, id uint) error {
	if err := db.Where("monitor_id = ?", id).Delete(&MonitorRun{}).Error; err != nil {
		return fmt.Errorf("delete runs: %w", err)
	}
	return db.Delete(&Monitor{}, id).Error
}

// GetMonitor retrieves a monitor by ID.
func GetMonitor(db *gorm.DB, id uint) (*Monitor, error) {
	var m Monitor
	if err := db.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMonitors returns all monitor rules.
func ListMonitors(db *gorm.DB) ([]Monitor, error) {
	var monitors []Monitor
	if err := db.Order("created_at DESC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// ListEnabledMonitors returns enabled monitors ordered by NextRunAt.
func ListEnabledMonitors(db *gorm.DB) ([]Monitor, error) {
	var monitors []Monitor
	if err := db.Where("enabled = ?", true).Order("next_run_at ASC").Find(&monitors).Error; err != nil {
		return nil, err
	}
	return monitors, nil
}

// CreateMonitorRun inserts a new run record.
func CreateMonitorRun(db *gorm.DB, run *MonitorRun) error {
	return db.Create(run).Error
}

// FinishMonitorRun updates a run with results.
func FinishMonitorRun(db *gorm.DB, run *MonitorRun) error {
	return db.Save(run).Error
}

// ListMonitorRuns returns the last N runs for a monitor, newest first.
func ListMonitorRuns(db *gorm.DB, monitorID uint, limit int) ([]MonitorRun, error) {
	if limit <= 0 {
		limit = 50
	}
	var runs []MonitorRun
	if err := db.Where("monitor_id = ?", monitorID).
		Order("started_at DESC").
		Limit(limit).
		Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// PruneMonitorRuns keeps only the last maxKeep runs per monitor.
func PruneMonitorRuns(db *gorm.DB, monitorID uint, maxKeep int) error {
	var count int64
	db.Model(&MonitorRun{}).Where("monitor_id = ?", monitorID).Count(&count)
	if count <= int64(maxKeep) {
		return nil
	}

	// Find the ID threshold for keeping.
	var keepMin MonitorRun
	if err := db.Where("monitor_id = ?", monitorID).
		Order("started_at DESC").
		Offset(maxKeep).
		First(&keepMin).Error; err != nil {
		return nil
	}

	return db.Where("monitor_id = ? AND started_at <= ?", monitorID, keepMin.StartedAt).
		Delete(&MonitorRun{}).Error
}

// IsSongDownloaded checks if a song with given (source, songID) already has
// a completed download task. Used for chart dedup.
func IsSongDownloaded(db *gorm.DB, source, songID string) bool {
	var count int64
	db.Model(&TaskRecord{}).
		Where("source = ? AND song_id = ? AND status = ?", source, songID, "done").
		Count(&count)
	return count > 0
}

// FilterDownloaded returns the set of song IDs that already have a completed
// download task for the given source. Used for batch dedup.
func FilterDownloaded(db *gorm.DB, source string, songIDs []string) map[string]struct{} {
	if len(songIDs) == 0 {
		return nil
	}

	var existing []string
	db.Model(&TaskRecord{}).
		Where("source = ? AND song_id IN ? AND status = ?", source, songIDs, "done").
		Pluck("song_id", &existing)

	result := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		result[id] = struct{}{}
	}
	return result
}

// UpdateMonitorSchedule updates LastRunAt and computes NextRunAt.
func UpdateMonitorSchedule(db *gorm.DB, m *Monitor) error {
	now := time.Now()
	m.LastRunAt = &now
	m.NextRunAt = now.Add(time.Duration(m.Interval) * time.Hour)
	return db.Model(m).Updates(map[string]any{
		"last_run_at": m.LastRunAt,
		"next_run_at": m.NextRunAt,
		"updated_at":  now,
	}).Error
}
