package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heart-rate-server/internal/config"
	"heart-rate-server/internal/handlers"
	"heart-rate-server/internal/middleware"
	"heart-rate-server/internal/models"
	"heart-rate-server/internal/storage"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func testApp(t *testing.T) (*handlers.App, *miniredis.Miniredis) {
	t.Helper()
	cfg := &config.Config{
		DBDSN: filepath.Join(t.TempDir(), "test.db"), BcryptCost: 4, TokenExpiry: 24 * time.Hour,
		CookieHashKey: bytes.Repeat([]byte{1}, 64), CookieBlockKey: bytes.Repeat([]byte{2}, 32),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	db, err := storage.InitDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr(), MaxRetries: -1})
	t.Cleanup(func() { client.Close() })
	return &handlers.App{DB: db, Redis: client, Config: cfg, SecureCookie: middleware.NewSecureCookie(cfg.CookieHashKey, cfg.CookieBlockKey)}, srv
}

func testUser(t *testing.T, app *handlers.App) (models.User, *http.Cookie) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), app.Config.BcryptCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: uuid.NewString(), Password: string(hash), UUID: uuid.NewString(), ReportToken: uuid.NewString()}
	if err := app.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := app.SecureCookie.SetAuthCookie(w, models.AuthInfo{UserID: user.ID, Username: user.Username, Expires: time.Now().Add(app.Config.TokenExpiry)}); err != nil {
		t.Fatal(err)
	}
	return user, w.Result().Cookies()[0]
}

func request(router http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func heartRateBody(rate int, measuredAt int64) string {
	return fmt.Sprintf(`{"data":{"heart_rate":%d},"measured_at":%d}`, rate, measuredAt)
}

func TestPublicUUIDCannotReportEvenWithWarmReadCache(t *testing.T) {
	app, srv := testApp(t)
	user, cookie := testUser(t, app)
	router := newRouter(app)
	view := request(router, "GET", "/uuid/widget/view/"+user.UUID, "", nil)
	if view.Code != 200 || strings.Contains(view.Body.String(), user.ReportToken) {
		t.Fatalf("public view leaked a token or failed: %d", view.Code)
	}
	if _, err := srv.Get("uuid_to_user_id:" + user.UUID); err != nil {
		t.Fatal("read cache was not populated")
	}
	body := heartRateBody(72, time.Now().UnixMilli())
	denied := request(router, "POST", "/uuid/"+user.UUID+"/receive_data", body, nil)
	if denied.Code != 404 || srv.Exists(fmt.Sprintf("heart_rate:%d", user.ID)) {
		t.Fatalf("public UUID could report: %d %s", denied.Code, denied.Body.String())
	}
	allowed := request(router, "POST", "/uuid/"+user.ReportToken+"/receive_data", body, nil)
	if allowed.Code != 200 {
		t.Fatalf("private token rejected: %d %s", allowed.Code, allowed.Body.String())
	}
	read := request(router, "GET", "/uuid/"+user.UUID+"/latest-heart-rate", "", nil)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"heart_rate":72`) {
		t.Fatalf("public read failed: %d %s", read.Code, read.Body.String())
	}
	if w := request(router, "GET", "/uuid", "", nil); w.Code != 401 {
		t.Fatalf("unauthenticated credential lookup returned %d", w.Code)
	}
	w := request(router, "GET", "/uuid", "", cookie)
	var response struct {
		Data struct {
			UUID        string `json:"uuid"`
			ReportToken string `json:"report_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || response.Data.UUID != user.UUID || response.Data.ReportToken != user.ReportToken {
		t.Fatal("authenticated credential lookup did not return both identities")
	}
}

func TestRegisterCreatesSeparateReportTokenAndLoginWorks(t *testing.T) {
	app, _ := testApp(t)
	router := newRouter(app)
	w := request(router, "POST", "/register", `{"username":"new-user","password":"test-password"}`, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("registration failed: %d %s", w.Code, w.Body.String())
	}
	var user models.User
	if err := app.DB.Where("username = ?", "new-user").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.ReportToken == "" || user.ReportToken == user.UUID {
		t.Fatal("new user lacks a separate report token")
	}
	w = request(router, "POST", "/login", `{"username":"new-user","password":"test-password"}`, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
}

func TestBothReportRoutesValidateInput(t *testing.T) {
	app, _ := testApp(t)
	user, cookie := testUser(t, app)
	router := newRouter(app)
	now := time.Now().UnixMilli()
	cases := []struct {
		name, body string
		status     int
	}{
		{"valid", heartRateBody(72, now), 200},
		{"valid_future", heartRateBody(250, now+int64((4*time.Minute)/time.Millisecond)), 200},
		{"overflow", heartRateBody(72, now+(int64(1)<<58)), 400},
		{"maximum", heartRateBody(72, math.MaxInt64), 400},
		{"minimum", heartRateBody(72, math.MinInt64), 400},
		{"old", heartRateBody(72, now-int64((11*time.Minute)/time.Millisecond)), 400},
		{"future", heartRateBody(72, now+int64((6*time.Minute)/time.Millisecond)), 400},
		{"zero_time", heartRateBody(72, 0), 400},
		{"missing_time", `{"data":{"heart_rate":72}}`, 400},
		{"zero_rate", heartRateBody(0, now), 400},
		{"excessive_rate", heartRateBody(251, now), 400},
		{"missing_data", fmt.Sprintf(`{"measured_at":%d}`, now), 400},
		{"malformed", `{`, 400},
	}
	for _, path := range []string{"/receive_data", "/uuid/" + user.ReportToken + "/receive_data"} {
		for _, tc := range cases {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				key := fmt.Sprintf("heart_rate:%d", user.ID)
				before := app.Redis.ZCard(context.Background(), key).Val()
				w := request(router, "POST", path, tc.body, cookie)
				if w.Code != tc.status {
					t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
				}
				if tc.status != 200 && app.Redis.ZCard(context.Background(), key).Val() != before {
					t.Fatal("invalid input changed stored data")
				}
			})
		}
	}
}

func TestReportsTrimHistoryAndPoisonedFutureScores(t *testing.T) {
	for _, retention := range []time.Duration{10 * time.Minute, 30 * time.Minute} {
		t.Run(retention.String(), func(t *testing.T) {
			app, srv := testApp(t)
			user, cookie := testUser(t, app)
			router := newRouter(app)
			path := "/receive_data"
			if retention == 30*time.Minute {
				path = "/uuid/" + user.ReportToken + "/receive_data"
				cookie = nil
			}
			key := fmt.Sprintf("heart_rate:%d", user.ID)
			now := time.Now()
			for member, score := range map[string]float64{
				"expired":  float64(now.Add(-retention - time.Second).UnixMilli()),
				"poisoned": float64(now.UnixMilli() + (int64(1) << 58)),
				heartRateBody(70, now.Add(-time.Minute).UnixMilli()): float64(now.Add(-time.Minute).UnixMilli()),
			} {
				if _, err := srv.ZAdd(key, score, member); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				w := request(router, "POST", path, heartRateBody(72, now.UnixMilli()), cookie)
				if w.Code != 200 {
					t.Fatalf("report failed: %d %s", w.Code, w.Body.String())
				}
				members, err := srv.ZMembers(key)
				if err != nil || len(members) != 2 {
					t.Fatalf("members = %v, error = %v", members, err)
				}
				if ttl := srv.TTL(key); ttl != retention {
					t.Fatalf("TTL = %v, want %v", ttl, retention)
				}
				if i == 0 {
					srv.FastForward(retention - time.Second)
				}
			}
			w := request(router, "GET", "/uuid/"+user.UUID+"/latest-heart-rate", "", nil)
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"heart_rate":72`) {
				t.Fatalf("poisoned data still shadows latest: %d %s", w.Code, w.Body.String())
			}
			srv.FastForward(retention + time.Second)
			if srv.Exists(key) {
				t.Fatal("inactive key did not expire")
			}
		})
	}
}

func TestNegativeCacheAvoidsRepeatedDatabaseQueries(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			app, srv := testApp(t)
			queries := 0
			if err := app.DB.Callback().Query().Before("gorm:query").Register("test:count", func(_ *gorm.DB) { queries++ }); err != nil {
				t.Fatal(err)
			}
			router := newRouter(app)
			path := "/uuid/missing/latest-heart-rate"
			if method == "POST" {
				path = "/uuid/missing/receive_data"
			}
			for i := 0; i < 2; i++ {
				w := request(router, method, path, "", nil)
				if w.Code != 404 {
					t.Fatalf("status = %d", w.Code)
				}
			}
			if queries != 1 {
				t.Fatalf("database queries = %d, want 1", queries)
			}
			srv.FastForward(5*time.Minute + time.Second)
			request(router, method, path, "", nil)
			if queries != 2 {
				t.Fatalf("expired negative cache did not recheck: %d", queries)
			}
		})
	}
}

func TestIdentityCacheDatabaseFailureReturns500(t *testing.T) {
	app, _ := testApp(t)
	if err := app.DB.Callback().Query().Before("gorm:query").Register("test:fail", func(tx *gorm.DB) { tx.AddError(errors.New("database unavailable")) }); err != nil {
		t.Fatal(err)
	}
	w := request(newRouter(app), "GET", "/uuid/missing/latest-heart-rate", "", nil)
	if w.Code != 500 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("database failure was hidden: %d %s", w.Code, w.Body.String())
	}
}

func TestGetUUIDErrorIsSingleJSONResponse(t *testing.T) {
	for _, failure := range []string{"deleted_user", "database_error"} {
		t.Run(failure, func(t *testing.T) {
			app, _ := testApp(t)
			user, cookie := testUser(t, app)
			if failure == "deleted_user" {
				if err := app.DB.Delete(&user).Error; err != nil {
					t.Fatal(err)
				}
			} else if err := app.DB.Callback().Query().Before("gorm:query").Register("test:fail", func(tx *gorm.DB) { tx.AddError(errors.New("database unavailable")) }); err != nil {
				t.Fatal(err)
			}
			w := request(newRouter(app), "GET", "/uuid", "", cookie)
			if w.Code != 500 || !json.Valid(w.Body.Bytes()) || strings.Contains(w.Body.String(), `"data"`) {
				t.Fatalf("invalid error response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRedisFailureIsNotReportedAsMissingData(t *testing.T) {
	app, srv := testApp(t)
	user, cookie := testUser(t, app)
	router := newRouter(app)
	publicPath := "/uuid/" + user.UUID + "/latest-heart-rate"
	if w := request(router, "GET", publicPath, "", nil); w.Code != 404 {
		t.Fatalf("healthy empty Redis returned %d", w.Code)
	}
	if w := request(router, "GET", "/latest-heart-rate", "", cookie); w.Code != 404 {
		t.Fatalf("authenticated empty Redis returned %d", w.Code)
	}
	srv.SetError("ERR backend unavailable")
	if w := request(router, "GET", publicPath, "", nil); w.Code != 503 {
		t.Fatalf("public backend failure returned %d: %s", w.Code, w.Body.String())
	}
	if w := request(router, "GET", "/latest-heart-rate", "", cookie); w.Code != 500 {
		t.Fatalf("authenticated backend failure returned %d", w.Code)
	}
}

func TestBothReadRoutesReturnLatestHeartRate(t *testing.T) {
	app, _ := testApp(t)
	user, cookie := testUser(t, app)
	router := newRouter(app)
	now := time.Now().UnixMilli()
	for i, rate := range []int{70, 72} {
		w := request(router, "POST", "/receive_data", heartRateBody(rate, now+int64(i)), cookie)
		if w.Code != 200 {
			t.Fatalf("report returned %d: %s", w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/latest-heart-rate", "/uuid/" + user.UUID + "/latest-heart-rate"} {
		w := request(router, "GET", path, "", cookie)
		var response struct {
			Data models.HeartRateDataResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || response.Data.HeartRate != 72 || response.Data.MeasuredAt != now+1 {
			t.Fatalf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
}

type deadlineHook struct {
	t          *testing.T
	cacheCalls int
	dataCalls  int
}

func (h *deadlineHook) check(ctx context.Context, cache bool) {
	h.t.Helper()
	deadline, ok := ctx.Deadline()
	limit := storage.RedisRequestTimeout
	if cache {
		h.cacheCalls++
		limit = time.Second
	} else {
		h.dataCalls++
	}
	if !ok || time.Until(deadline) > limit || time.Until(deadline) <= 0 {
		h.t.Fatalf("Redis command lacks a live deadline within %v", limit)
	}
}

func (h *deadlineHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	h.check(ctx, cmd.Name() == "get" || cmd.Name() == "set")
	return ctx, errors.New("backend unavailable")
}

func (h *deadlineHook) AfterProcess(context.Context, redis.Cmder) error { return nil }

func (h *deadlineHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	h.check(ctx, false)
	return ctx, errors.New("backend unavailable")
}

func (h *deadlineHook) AfterProcessPipeline(context.Context, []redis.Cmder) error { return nil }

func TestRedisCacheAndDataCommandsHaveBoundedDeadlines(t *testing.T) {
	app, _ := testApp(t)
	user, cookie := testUser(t, app)
	hook := &deadlineHook{t: t}
	app.Redis.AddHook(hook)
	router := newRouter(app)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/uuid/" + user.UUID + "/latest-heart-rate", 503},
		{"GET", "/latest-heart-rate", 500},
		{"POST", "/receive_data", 500},
		{"POST", "/uuid/" + user.ReportToken + "/receive_data", 500},
	} {
		w := request(router, tc.method, tc.path, heartRateBody(72, time.Now().UnixMilli()), cookie)
		if w.Code != tc.status {
			t.Fatalf("%s returned %d, want %d", tc.path, w.Code, tc.status)
		}
	}
	if hook.cacheCalls == 0 || hook.dataCalls != 4 {
		t.Fatalf("deadline checks covered %d cache calls and %d data calls", hook.cacheCalls, hook.dataCalls)
	}
}
