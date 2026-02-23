package testutil

import (
	"path/filepath"
	"testing"

	"cgwm/shelfy/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func NewSQLiteRepo(t *testing.T) *repo.GormRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := repo.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo.NewGormRepository(db)
}
