package repo

import (
	"fmt"

	"cgwm/shelfy/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func OpenDB(cfg config.Config) (*gorm.DB, error) {
	switch cfg.DBDriver {
	case "sqlite":
		return gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	case "mariadb", "mysql":
		return gorm.Open(mysql.Open(cfg.DBDSN), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.DBDriver)
	}
}
