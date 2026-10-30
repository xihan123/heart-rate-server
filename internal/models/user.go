package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username    string `gorm:"unique;not null"`
	Password    string `gorm:"not null"`
	UUID        string `gorm:"uniqueIndex;size:36"`
	ReportToken string `gorm:"uniqueIndex;size:36"`
}

type AuthInfo struct {
	UserID   uint      `json:"user_id"`
	Username string    `json:"username"`
	Expires  time.Time `json:"expires"`
}

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type HeartRateData struct {
	Data struct {
		HeartRate int `json:"heart_rate" validate:"required,min=1,max=250"`
	} `json:"data"`
	MeasuredAt int64 `json:"measured_at" validate:"required"`
}

type HeartRateDataResponse struct {
	HeartRate  int   `json:"heart_rate"`
	MeasuredAt int64 `json:"measured_at"`
}

type Response struct {
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}
