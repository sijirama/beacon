package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/sijirama/beacon/internal/auth"
	"github.com/sijirama/beacon/internal/config"
	"github.com/sijirama/beacon/internal/db"
	"github.com/sijirama/beacon/internal/handlers"
	"github.com/sijirama/beacon/internal/middleware"
	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/queue"
	"github.com/sijirama/beacon/internal/services/email"
	"github.com/sijirama/beacon/internal/web"
)

func main() {
	cfg := config.Load()

	// --- DB ---
	gormDB, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: open: %v", err)
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		log.Fatalf("db: migrate: %v", err)
	}
	sqlDB, _ := gormDB.DB()
	defer sqlDB.Close()

	// --- Redis (sessions + magic links) ---
	store, err := auth.NewStore(cfg.RedisURL)
	if err != nil {
		log.Fatalf("auth store: %v", err)
	}
	defer store.Close()
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := store.Ping(pingCtx); err != nil {
		cancelPing()
		log.Fatalf("redis: %v", err)
	}
	cancelPing()

	// --- Asynq client + inspector ---
	qClient, err := queue.NewClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("queue client: %v", err)
	}
	defer qClient.Close()
	if err := qClient.Ping(); err != nil {
		log.Fatalf("queue ping: %v", err)
	}
	inspector, err := queue.NewInspector(cfg.RedisURL)
	if err != nil {
		log.Fatalf("queue inspector: %v", err)
	}
	defer inspector.Close()

	// --- Email ---
	sender := email.NewSender(cfg.ResendAPIKey, cfg.EmailFrom)
	emailTpls, err := email.LoadTemplates("web/templates/email")
	if err != nil {
		log.Fatalf("email templates: %v", err)
	}
	notifier := email.NewEmailNotifier(sender, emailTpls, cfg.EmailTo)

	// --- Asynq worker (in-process) ---
	worker, mux, err := queue.NewWorker(cfg.RedisURL, queue.WorkerDeps{
		DB:          gormDB,
		Notifier:    notifier,
		Concurrency: 5,
	})
	if err != nil {
		log.Fatalf("worker: %v", err)
	}
	if err := worker.Start(mux); err != nil {
		log.Fatalf("worker start: %v", err)
	}
	defer worker.Shutdown()

	// --- Templates ---
	renderer, err := web.NewRenderer("web/templates", web.DefaultFuncs())
	if err != nil {
		log.Fatalf("renderer: %v", err)
	}

	// --- Handlers ---
	authH := handlers.NewAuth(cfg, store, sender, renderer)
	pagesH := handlers.NewPages(gormDB, renderer, inspector)
	tokensH := handlers.NewTokens(gormDB)
	emitH := handlers.NewEmit(gormDB, qClient, cfg.EmailRetries)
	systemH := handlers.NewSystem(cfg.BaseURL, sqlDB, store, qClient)
	emitLimiter := middleware.NewFixedWindowLimiter(60, time.Minute)
	loginLimiter := middleware.NewFixedWindowLimiter(5, 15*time.Minute)

	// --- Router ---
	r := gin.Default()
	r.Static("/static", "./web/static")
	r.GET("/health", systemH.Health)
	r.GET("/heartbeat", systemH.Heartbeat)
	r.GET("/api-spec", systemH.APISpec)
	r.GET("/openapi.json", systemH.OpenAPI)

	// API: bearer-authenticated
	api := r.Group(
		"/",
		middleware.RequireBearer(gormDB),
		middleware.LimitJSON(emitLimiter, func(c *gin.Context) string {
			if v, ok := c.Get("token"); ok {
				if t, ok := v.(*models.Token); ok && t != nil {
					return fmt.Sprintf("emit:token:%d", t.ID)
				}
			}
			return "emit:ip:" + c.ClientIP()
		}),
	)
	api.POST("/emit", emitH.Post)

	sessionDeps := middleware.SessionDeps{Store: store, SessionTTL: cfg.SessionTTL}

	// Public web auth flows
	pub := r.Group("/", middleware.LoadSession(sessionDeps))
	pub.GET("/login", authH.GetLogin)
	pub.POST("/login", middleware.LimitHTMLRedirect(loginLimiter, func(c *gin.Context) string {
		email := strings.TrimSpace(strings.ToLower(c.PostForm("email")))
		return "login:" + c.ClientIP() + ":" + email
	}, "/login", "Too many sign-in attempts. Try again in a bit."), authH.PostLogin)
	pub.GET("/auth/verify", authH.GetVerify)
	pub.POST("/logout", authH.PostLogout)

	// Protected web UI
	protected := r.Group("/", middleware.RequireSession(sessionDeps))
	protected.GET("/", pagesH.Events)
	protected.GET("/events/:id", pagesH.EventDetail)
	protected.GET("/queue", pagesH.Queue)
	protected.GET("/tokens", pagesH.Tokens)
	protected.POST("/tokens", tokensH.Create)
	protected.POST("/tokens/:id/delete", tokensH.Delete)

	// --- HTTP server ---
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("beacon listening on :%s (base_url=%s)", cfg.Port, cfg.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
