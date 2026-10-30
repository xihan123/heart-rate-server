package storage

import (
	"fmt"

	"heart-rate-server/internal/config"
	"heart-rate-server/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func InitDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %v", err)
	}

	if err := db.AutoMigrate(&models.User{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %v", err)
	}

	// Keep existing public UUIDs when assigning private report tokens.
	var users []models.User
	if err := db.Select("id").Where("report_token IS NULL OR report_token = ?", "").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to find accounts needing report tokens: %w", err)
	}
	for _, user := range users {
		if err := db.Model(&models.User{}).Where("id = ? AND (report_token IS NULL OR report_token = ?)", user.ID, "").Update("report_token", uuid.NewString()).Error; err != nil {
			return nil, fmt.Errorf("failed to migrate report token for user %d: %w", user.ID, err)
		}
	}

	return db, nil
}
