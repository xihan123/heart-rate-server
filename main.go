package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"heart-rate-server/internal/config"
	"heart-rate-server/internal/handlers"
	"heart-rate-server/internal/middleware"
	"heart-rate-server/internal/storage"

	"github.com/gorilla/mux"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	db, err := storage.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	redisClient, err := storage.InitRedis(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	secureCookie := middleware.NewSecureCookie(cfg.CookieHashKey, cfg.CookieBlockKey)

	app := &handlers.App{
		DB:           db,
		Redis:        redisClient,
		Config:       cfg,
		SecureCookie: secureCookie,
	}

	r := newRouter(app)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}

func newRouter(app *handlers.App) *mux.Router {
	r := mux.NewRouter()
	r.PathPrefix("/static/").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))),
	)
	r.Use(middleware.RecoveryMiddleware)
	r.Use(middleware.JSONContentTypeMiddleware)

	r.HandleFunc("/", app.IndexHandler).Methods("GET")
	r.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	r.HandleFunc("/register", app.RegisterHandler).Methods("POST")
	r.HandleFunc("/login", app.LoginHandler).Methods("POST")

	uuidRouter := r.PathPrefix("/uuid").Subrouter()
	uuidRouter.Use(middleware.NewUUIDCacheMiddleware(app.DB, app.Redis).Handler)
	uuidRouter.HandleFunc("/{report_token}/receive_data", app.UUIDReportDataHandler).Methods("POST")
	uuidRouter.HandleFunc("/{uuid}/latest-heart-rate", app.PublicHeartRateHandler).Methods("GET")
	uuidRouter.HandleFunc("/widget/view/{uuid}", app.PublicHeartRateHTMLHandler).Methods("GET")

	authRouter := r.PathPrefix("").Subrouter()
	authRouter.Use(middleware.AuthMiddleware(app.SecureCookie, app.Config))
	authRouter.HandleFunc("/receive_data", app.ReceiveDataHandler).Methods("POST")
	authRouter.HandleFunc("/latest-heart-rate", app.LatestHeartRateHandler).Methods("GET")
	authRouter.HandleFunc("/uuid", app.GetUUIDHandler).Methods("GET")
	authRouter.HandleFunc("/logout", app.LogoutHandler).Methods("POST")
	return r
}
