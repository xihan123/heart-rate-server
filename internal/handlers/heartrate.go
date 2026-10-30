package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"heart-rate-server/internal/models"
	"heart-rate-server/internal/storage"
	"heart-rate-server/internal/utils"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

func (app *App) ReceiveDataHandler(w http.ResponseWriter, r *http.Request) {
	authInfo := r.Context().Value("authInfo").(*models.AuthInfo)
	app.storeHeartRate(w, r, authInfo.UserID, 10*time.Minute)
}

func (app *App) UUIDReportDataHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("cached_user_id").(uint)
	if !ok {
		utils.SendError(w, http.StatusBadRequest, nil, "Missing user identification")
		return
	}
	app.storeHeartRate(w, r, userID, 30*time.Minute)
}

func (app *App) storeHeartRate(w http.ResponseWriter, r *http.Request, userID uint, retention time.Duration) {
	var data models.HeartRateData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		utils.SendError(w, http.StatusBadRequest, err, "Invalid request body")
		return
	}

	if err := validate.Struct(data); err != nil {
		utils.SendError(w, http.StatusBadRequest, err, "Validation failed")
		return
	}

	now := time.Now()

	// Compare milliseconds directly so untrusted timestamps cannot overflow nanoseconds.
	if data.MeasuredAt > now.Add(5*time.Minute).UnixMilli() {
		utils.SendError(w, http.StatusBadRequest, nil, "Measurement time cannot be in the future")
		return
	}

	if data.MeasuredAt < now.Add(-10*time.Minute).UnixMilli() {
		utils.SendError(w, http.StatusBadRequest, nil, "Measurement time is too old")
		return
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to process data")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storage.RedisRequestTimeout)
	defer cancel()
	key := fmt.Sprintf("heart_rate:%d", userID)
	z := &redis.Z{
		Score:  float64(data.MeasuredAt),
		Member: jsonData,
	}

	pipe := app.Redis.TxPipeline()
	pipe.ZAdd(ctx, key, z)
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("(%d", now.Add(-retention).UnixMilli()))
	pipe.ZRemRangeByScore(ctx, key, fmt.Sprintf("(%d", now.Add(5*time.Minute).UnixMilli()), "+inf")
	pipe.Expire(ctx, key, retention)
	_, err = pipe.Exec(ctx)
	if err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to store data")
		return
	}

	utils.SendResponse(w, http.StatusOK, "OK", nil)
}

func (app *App) LatestHeartRateHandler(w http.ResponseWriter, r *http.Request) {
	authInfo := r.Context().Value("authInfo").(*models.AuthInfo)
	ctx, cancel := context.WithTimeout(r.Context(), storage.RedisRequestTimeout)
	defer cancel()
	key := fmt.Sprintf("heart_rate:%d", authInfo.UserID)

	result, err := app.Redis.ZRevRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to retrieve data")
		return
	}

	if len(result) == 0 {
		utils.SendError(w, http.StatusNotFound, nil, "No heart rate data found")
		return
	}

	var storedData models.HeartRateData
	if err := json.Unmarshal([]byte(result[0].Member.(string)), &storedData); err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to parse data")
		return
	}
	var resultData models.HeartRateDataResponse
	resultData.HeartRate = storedData.Data.HeartRate
	resultData.MeasuredAt = storedData.MeasuredAt

	utils.SendResponse(w, http.StatusOK, "ok", resultData)
}

func (app *App) PublicHeartRateHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("cached_user_id").(uint)
	if !ok {
		utils.SendError(w, http.StatusBadRequest, nil, "Missing user identification")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storage.RedisRequestTimeout)
	defer cancel()
	key := fmt.Sprintf("heart_rate:%d", userID)

	result, err := app.Redis.ZRevRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil {
		utils.SendError(w, http.StatusServiceUnavailable, err, "Failed to retrieve data")
		return
	}
	if len(result) == 0 {
		utils.SendError(w, http.StatusNotFound, nil, "No heart rate data available")
		return
	}

	var data models.HeartRateData
	if err := json.Unmarshal([]byte(result[0].Member.(string)), &data); err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to parse data")
		return
	}
	var resultData models.HeartRateDataResponse
	resultData.HeartRate = data.Data.HeartRate
	resultData.MeasuredAt = data.MeasuredAt

	utils.SendResponse(w, http.StatusOK, "ok", resultData)
}

func (app *App) PublicHeartRateHTMLHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]

	tmpl, err := template.ParseFiles("templates/default.html")
	if err != nil {
		utils.SendError(w, http.StatusInternalServerError, err, "Failed to load template")
		return
	}

	w.Header().Set("Content-Type", "text/html")
	err2 := tmpl.Execute(w, map[string]interface{}{
		"UUID": uuid,
	})
	if err2 != nil {
		return
	}
}
