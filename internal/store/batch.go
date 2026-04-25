package store

import (
	"time"

	"gorm.io/gorm"
)

// BatchRecord is the GORM model for the download_batches table.
type BatchRecord struct {
	ID        string    `gorm:"primaryKey"`
	Source    string    `gorm:"not null"`
	Name      string    `gorm:"not null"`
	Total     int       `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName overrides the default table name.
func (BatchRecord) TableName() string { return "download_batches" }

// CreateBatch inserts a new batch record.
func CreateBatch(db *gorm.DB, id, source, name string, total int) error {
	now := time.Now()
	return db.Create(&BatchRecord{
		ID:        id,
		Source:    source,
		Name:      name,
		Total:     total,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}

// GetBatch retrieves a batch by ID.
func GetBatch(db *gorm.DB, id string) (*BatchRecord, error) {
	var r BatchRecord
	if err := db.First(&r, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// ListBatchNames returns a map of batchID -> name for all batches.
// Used at startup to restore batch names into the download Manager.
func ListBatchNames(db *gorm.DB) (map[string]string, error) {
	var records []BatchRecord
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}
	names := make(map[string]string, len(records))
	for _, r := range records {
		names[r.ID] = r.Name
	}
	return names, nil
}

// BatchWithStats represents a batch with aggregated task counts from the DB.
type BatchWithStats struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Failed  int    `json:"failed"`
	Running int    `json:"running"`
	Pending int    `json:"pending"`
}

// ListBatchesWithStats aggregates task counts per batch from the DB.
// This is the persistent source of truth, surviving restarts.
func ListBatchesWithStats(db *gorm.DB) ([]BatchWithStats, error) {
	type row struct {
		BatchID string
		Name    string
		Total   int
		Done    int
		Failed  int
		Running int
		Pending int
	}
	var rows []row
	err := db.Raw(`
		SELECT
			b.id AS batch_id,
			b.name,
			COUNT(t.id) AS total,
			SUM(CASE WHEN t.status = 'done' THEN 1 ELSE 0 END) AS done,
			SUM(CASE WHEN t.status = 'failed' THEN 1 ELSE 0 END) AS failed,
			SUM(CASE WHEN t.status = 'running' THEN 1 ELSE 0 END) AS running,
			SUM(CASE WHEN t.status = 'pending' THEN 1 ELSE 0 END) AS pending
		FROM download_batches b
		LEFT JOIN download_tasks t ON t.batch_id = b.id
		GROUP BY b.id
		ORDER BY b.created_at DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]BatchWithStats, len(rows))
	for i, r := range rows {
		result[i] = BatchWithStats{
			ID:      r.BatchID,
			Name:    r.Name,
			Total:   r.Total,
			Done:    r.Done,
			Failed:  r.Failed,
			Running: r.Running,
			Pending: r.Pending,
		}
	}
	return result, nil
}
