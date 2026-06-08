package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/handler"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/repository"
	"github.com/go-chi/chi/v5"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	logger     *slog.Logger
	cfg        *config.Config
}

// New creates and configures a new Server.
func New(cfg *config.Config, logger *slog.Logger, db *repository.DB, version, gitCommit, buildTime, goVer string) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger(logger))
	r.Use(middleware.ErrorHandler())

	// Repositories
	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	// Allowed roots for file access (path traversal protection)
	allowedRoots := strings.Split(os.Getenv("FYOM_MEDIA_ROOTS"), ":")

	// Handlers
	healthHandler := handler.NewHealthHandler(version, gitCommit, buildTime, goVer)
	mediaHandler := handler.NewMediaHandler(db, mediaRepo, jobRepo)
	mediaHandler.SetAllowedRoots(allowedRoots)
	authHandler := handler.NewAuthHandler(userRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	// Public routes
	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)
	r.Post("/api/v1/auth/register", authHandler.Register)
	r.Post("/api/v1/auth/login", authHandler.Login)

	// Protected routes (require auth)
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Post("/api/v1/library/import", mediaHandler.Import)
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
		r.Delete("/api/v1/library/{id}", mediaHandler.Delete)
		r.Get("/api/v1/media/{id}/stream", mediaHandler.Stream)
		r.Get("/api/v1/media/{id}/poster", mediaHandler.Poster)
		r.Get("/api/v1/auth/me", authHandler.Me)
	})

	// Static files (embedded frontend in production)
	r.Handle("/assets/*", http.StripPrefix("/assets", http.FileServer(http.Dir("./web/dist/assets"))))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/dist/index.html")
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/dist/index.html")
	})

	httpServer := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		router:     r,
		logger:     logger,
		cfg:        cfg,
	}
}

// Run starts the server and blocks until shutdown.
func (s *Server) Run() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s.logger.Info("server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	<-quit
	s.logger.Info("server shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}
