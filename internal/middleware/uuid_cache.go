package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"heart-rate-server/internal/models"
	"heart-rate-server/internal/utils"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

type UUIDCacheMiddleware struct {
	DB    *gorm.DB
	Redis *redis.Client
}

func NewUUIDCacheMiddleware(db *gorm.DB, redis *redis.Client) *UUIDCacheMiddleware {
	return &UUIDCacheMiddleware{
		DB:    db,
		Redis: redis,
	}
}

func (m *UUIDCacheMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		identifier, ok := vars["report_token"]
		column := "report_token"
		if !ok {
			identifier, ok = vars["uuid"]
			column = "uuid"
		}
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		// Leave the data request time to run when Redis is unavailable.
		cacheCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		cacheKey := fmt.Sprintf("%s_to_user_id:%s", column, identifier)

		var userID uint
		if cached, err := m.Redis.Get(cacheCtx, cacheKey).Result(); err == nil {
			if cached == "null" {
				utils.SendError(w, http.StatusNotFound, nil, "Unknown user identification")
				return
			}
			if cachedID, err := strconv.ParseUint(cached, 10, strconv.IntSize); err == nil && cachedID > 0 {
				userID = uint(cachedID)
				ctx = context.WithValue(ctx, "cached_user_id", userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		var user struct {
			ID uint
		}
		if err := m.DB.WithContext(ctx).Model(&models.User{}).Select("id").Where(column+" = ?", identifier).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				m.Redis.Set(cacheCtx, cacheKey, "null", 5*time.Minute)
				utils.SendError(w, http.StatusNotFound, nil, "Unknown user identification")
			} else {
				utils.SendError(w, http.StatusInternalServerError, err, "Database error")
			}
			return
		}

		userID = user.ID
		m.Redis.Set(cacheCtx, cacheKey, userID, time.Hour)
		ctx = context.WithValue(ctx, "cached_user_id", userID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
