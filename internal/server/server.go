// Package server wires up the HTTP server and static file serving.
package server

import (
	"context"
	"fmt"
	"io/fs"
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
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/fyom/fyom/web"
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
	settingRepo := repository.NewSystemSettingRepository(db)

	// Presigned URL signer — used by LocalProvider to generate URLs and by
	// middleware to validate them on media-serving endpoints.
	signer := presign.NewSigner(cfg.Auth.JWTSecret, 3600)

	// Create provider registry and register LocalProvider.
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))

	// Load configurable providers from database and register enabled ones.
	// TODO(phase4): FromRecord will instantiate S3Provider once implemented.
	providerRepo := repository.NewProviderRepository(db)
	if records, err := providerRepo.ListEnabled(context.Background()); err != nil {
		logger.Warn("failed to load providers from database", "err", err)
	} else {
		for _, rec := range records {
			p, err := provider.FromRecord(rec, signer)
			if err != nil {
				logger.Warn("skipping provider: unsupported type or invalid config",
					"provider_id", rec.ID,
					"type",        rec.Type,
					"err",         err,
				)
				continue
			}
			reg.Register(p)
			logger.Info("provider registered", "id", rec.ID, "type", rec.Type)
		}
	}

	// Handlers
	healthHandler := handler.NewHealthHandler(version, gitCommit, buildTime, goVer)
	mediaHandler := handler.NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, logger)
	// TODO: re-add path restriction when multi-user mode is implemented
	authHandler := handler.NewAuthHandler(userRepo, settingRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)
	systemHandler := handler.NewSystemHandler(settingRepo, authHandler.GetAuthService())
	adminProviderHandler := handler.NewAdminProviderHandler(providerRepo, logger)

	// ── Public API routes (no auth) ───────────────────────────────────────
	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)
	r.Get("/api/v1/system/status", systemHandler.Status)
	r.Post("/api/v1/system/initialize", systemHandler.Initialize)
	r.Post("/api/v1/auth/register", authHandler.Register)
	r.Post("/api/v1/auth/login", authHandler.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library/{id}/episodes", mediaHandler.ListEpisodes)
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
		r.Delete("/api/v1/library/{id}", mediaHandler.Delete)
		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Put("/api/v1/auth/me/password", authHandler.ChangePassword)

		// Admin-only routes
		r.With(middleware.RequireAdmin).Post("/api/v1/library/import", mediaHandler.Import)
		r.With(middleware.RequireAdmin).Get("/api/v1/admin/providers", adminProviderHandler.ListProviders)
		r.With(middleware.RequireAdmin).Post("/api/v1/admin/providers", adminProviderHandler.CreateProvider)
		r.With(middleware.RequireAdmin).Put("/api/v1/admin/providers/{id}", adminProviderHandler.UpdateProvider)
		r.With(middleware.RequireAdmin).Delete("/api/v1/admin/providers/{id}", adminProviderHandler.DeleteProvider)
	})

	// ── Presigned media endpoints (no JWT, sig-based auth) ─────────────────
	// <img> and <video> tags hit these directly via presigned URLs.
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.RequireValidPresign(signer))
		r.Get("/{id}/poster", mediaHandler.Poster)
		r.Get("/{id}/backdrop", mediaHandler.ServeBackdrop)
		r.Get("/{id}/stream", mediaHandler.Stream)
	})

	// ── Static files (embedded frontend) ───────────────────────────────────
	// Subscribe to the embedded FS, stripping the "dist" prefix so that
	// assets/js/index-CbS__aBa.js is served at /assets/js/index-CbS__aBa.js.
	staticFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// embed.FS.Sub only returns an error for invalid dir names;
		// "dist" is a verified subdirectory of the embed, so this is
		// effectively unreachable.  Log and fall back to a filesystem that
		// will simply return 404 for every path, allowing the SPA fallback
		// below to serve index.html — the app still boots, just without
		// static assets.
		logger.Error("failed to open embedded static FS", "error", err)
		staticFS = os.DirFS("/dev/null") // empty fallback
	}
	fileServer := http.FileServer(http.FS(staticFS))

	// Catch-all: serve static files, falling back to index.html for SPA routes.
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the requested file from the embedded FS.
		// If it doesn't exist, fall through to index.html (SPA fallback).
		f, err := staticFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			// File not found — serve index.html for client-side routing
			serveIndexHTML(w, r, staticFS)
			return
		}
		stat, err := f.Stat()
		_ = f.Close()
		if err != nil {
			serveIndexHTML(w, r, staticFS)
			return
		}
		// If the path is a directory, also fall back to index.html
		if stat.IsDir() {
			serveIndexHTML(w, r, staticFS)
			return
		}
		// File exists — serve it
		fileServer.ServeHTTP(w, r)
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

// serveIndexHTML reads index.html from the embedded FS and writes it.
func serveIndexHTML(w http.ResponseWriter, _ *http.Request, staticFS fs.FS) {
	data, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
