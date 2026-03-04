package db

import (
	"fmt"
	"strings"

	"emoji/internal/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg config.Config) (*gorm.DB, error) {
	parts := []string{
		fmt.Sprintf("host=%s", cfg.DBHost),
		fmt.Sprintf("user=%s", cfg.DBUser),
	}
	if cfg.DBPassword != "" {
		parts = append(parts, fmt.Sprintf("password=%s", cfg.DBPassword))
	}
	parts = append(parts,
		fmt.Sprintf("dbname=%s", cfg.DBName),
		fmt.Sprintf("port=%d", cfg.DBPort),
		fmt.Sprintf("sslmode=%s", cfg.DBSSLMode),
		fmt.Sprintf("TimeZone=%s", cfg.DBTimezone),
	)
	dsn := strings.Join(parts, " ")

	gcfg := &gorm.Config{}
	if cfg.Env != "prod" {
		gcfg.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(postgres.Open(dsn), gcfg)
	if err != nil {
		return nil, err
	}
	if cfg.DBSearchPath != "" {
		_ = db.Exec("SET search_path TO " + cfg.DBSearchPath).Error
	}
	return db, nil
}
