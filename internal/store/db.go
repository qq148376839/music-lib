package store

import (
	"fmt"
	"os"
	"path/filepath"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init opens (or creates) the SQLite database at dataDir/music-lib.db,
// runs AutoMigrate for all tables, and returns the *gorm.DB handle.
func Init(dataDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "music-lib.db")

	db, err := gorm.Open(glebarez.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.AutoMigrate(&BatchRecord{}, &TaskRecord{}, &Monitor{}, &MonitorRun{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return db, nil
}
