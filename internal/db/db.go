package db

import (
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sijirama/beacon/internal/models"
)

func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		_ = mkdir(dir)
	}
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}

func AutoMigrate(g *gorm.DB) error {
	return g.AutoMigrate(
		&models.Token{},
		&models.Event{},
		&models.Delivery{},
	)
}
