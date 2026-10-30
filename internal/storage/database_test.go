package storage

import (
	"path/filepath"
	"testing"

	"heart-rate-server/internal/config"
	"heart-rate-server/internal/models"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyUser struct {
	gorm.Model
	Username string `gorm:"unique;not null"`
	Password string `gorm:"not null"`
	UUID     string `gorm:"uniqueIndex;size:36"`
}

func (legacyUser) TableName() string { return "users" }

func TestInitDBMigratesExistingReportTokens(t *testing.T) {
	cfg := &config.Config{DBDSN: filepath.Join(t.TempDir(), "legacy.db")}
	legacy, err := gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.AutoMigrate(&legacyUser{}); err != nil {
		t.Fatal(err)
	}
	original := []legacyUser{
		{Username: "first", Password: "hash", UUID: uuid.NewString()},
		{Username: "second", Password: "hash", UUID: uuid.NewString()},
	}
	if err := legacy.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	tokens := map[uint]string{}
	for run := 0; run < 2; run++ {
		db, err := InitDB(cfg)
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { sqlDB.Close() })
		var users []models.User
		if err := db.Order("id").Find(&users).Error; err != nil {
			t.Fatal(err)
		}
		if len(users) != len(original) {
			t.Fatalf("users = %d", len(users))
		}
		for i, user := range users {
			if user.UUID != original[i].UUID || user.ReportToken == user.UUID {
				t.Fatal("migration changed public identity or reused it for writes")
			}
			if _, err := uuid.Parse(user.ReportToken); err != nil {
				t.Fatalf("invalid report token: %v", err)
			}
			if run == 0 {
				tokens[user.ID] = user.ReportToken
			} else if tokens[user.ID] != user.ReportToken {
				t.Fatal("restart rotated an existing report token")
			}
		}
		if users[0].ReportToken == users[1].ReportToken {
			t.Fatal("accounts share a report token")
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
