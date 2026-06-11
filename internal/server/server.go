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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/handler"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/fyom/fyom/web"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	httpServer  *http.Server
	router      *chi.Mux
	logger      *slog.Logger
	cfg         *config.Config
	db          *repository.DB
	libRepo     *repository.LibraryRepository
	settingRepo *repository.SystemSettingRepository
	mediaRepo   *repository.MediaRepository
}

func New(cfg *config.Config, logger *slog.Logger, db *repository.DB, version, gitCommit, buildTime, goVer string) *Server {
	r := chi.NewRouter()
	r.Use(middleware.Logger(logger))
	r.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)

	signer := presign.NewSigner(cfg.Auth.JWTSecret, 3600)
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))

	providerRepo := repository.NewProviderRepository(db)
	if records, err := providerRepo.ListEnabled(context.Background()); err != nil {
		logger.Warn("failed to load providers from database", "err", err)
	} else {
		for _, rec := range records {
			p, err := provider.FromRecord(rec, signer)
			if err != nil {
				logger.Warn("skipping provider", "provider_id", rec.ID, "err", err)
				continue
			}
			reg.Register(p)
			logger.Info("provider registered", "id", rec.ID, "type", rec.Type)
		}
	}

	adminRepo := repository.NewAdminRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	statusRepo := repository.NewUserMediaStatusRepository(db)

	healthHandler := handler.NewHealthHandler(version, gitCommit, buildTime, goVer)
	mediaHandler := handler.NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, libRepo, statusRepo, logger)
	authHandler := handler.NewAuthHandler(userRepo, libPermRepo, settingRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)
	systemHandler := handler.NewSystemHandler(settingRepo, authHandler.GetAuthService())
	adminProviderHandler := handler.NewAdminProviderHandler(providerRepo, logger)
	adminHandler := handler.NewAdminHandler(adminRepo, mediaRepo, settingRepo, libPermRepo, db)
	adminLibHandler := handler.NewAdminLibraryHandler(libRepo, providerRepo, libPermRepo)

	// Public routes
	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)
	r.Get("/api/v1/system/status", systemHandler.Status)
	r.Post("/api/v1/system/initialize", systemHandler.Initialize)
	r.Post("/api/v1/auth/register", authHandler.Register)
	r.Post("/api/v1/auth/login", authHandler.Login)

	// User-facing routes (auth + permissions)
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Use(middleware.ResolvePermissions(libPermRepo))
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library/{id}/episodes", mediaHandler.ListEpisodes)
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/continue", mediaHandler.GetContinueWatching)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
		r.Get("/api/v1/libraries", mediaHandler.GetLibraries)
		r.Put("/api/v1/media/{id}/progress", mediaHandler.UpdateProgress)
		r.Put("/api/v1/media/{id}/status", mediaHandler.SetStatus)
		r.Get("/api/v1/media/{id}/status", mediaHandler.GetStatus)
		r.Get("/api/v1/library/by-status", mediaHandler.GetByStatus)
		r.Delete("/api/v1/library/{id}", mediaHandler.Delete)
		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Put("/api/v1/auth/me/password", authHandler.ChangePassword)
		r.With(middleware.RequireAdmin).Post("/api/v1/library/import", mediaHandler.Import)
	})

	// Admin routes
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Use(middleware.RequireAdmin)
		r.Get("/stats", adminHandler.GetStats)
		r.Get("/import-jobs", adminHandler.ListImportJobs)
		r.Get("/settings", adminHandler.GetSettings)
		r.Put("/settings", adminHandler.UpdateSettings)
		r.Get("/media", adminHandler.ListMedia)
		r.Delete("/media/{id}", adminHandler.DeleteMedia)
		r.Get("/libraries", adminLibHandler.List)
		r.Post("/libraries", adminLibHandler.Create)
		r.Get("/libraries/{id}", adminLibHandler.Get)
		r.Put("/libraries/{id}", adminLibHandler.Update)
		r.Delete("/libraries/{id}", adminLibHandler.Delete)
		r.Delete("/libraries/{id}/items", adminLibHandler.DeleteLibraryWithItems)
		r.Post("/libraries/{id}/refresh", adminLibHandler.Refresh)
		r.Post("/libraries/{id}/check-missing", adminLibHandler.CheckMissing)
		r.Get("/media/missing", adminHandler.ListMissing)
		r.Delete("/media/missing", adminHandler.DeleteMissing)
		r.Get("/permissions", adminHandler.ListPermissions)
		r.Put("/permissions", adminHandler.UpdatePermission)
		r.Get("/providers", adminProviderHandler.ListProviders)
		r.Post("/providers", adminProviderHandler.CreateProvider)
		r.Put("/providers/{id}", adminProviderHandler.UpdateProvider)
		r.Delete("/providers/{id}", adminProviderHandler.DeleteProvider)
	})

	// Presigned media endpoints
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.RequireValidPresign(signer))
		r.Get("/{id}/poster", mediaHandler.Poster)
		r.Get("/{id}/backdrop", mediaHandler.ServeBackdrop)
		r.Get("/{id}/stream", mediaHandler.Stream)
	})

	// Static files
	staticFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		logger.Error("failed to open embedded static FS", "error", err)
		staticFS = os.DirFS("/dev/null")
	}
	fileServer := http.FileServer(http.FS(staticFS))

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		f, err := staticFS.Open(strings.TrimPrefix(path, "/"))
		if err != nil {
			serveIndexHTML(w, r, staticFS)
			return
		}
		stat, err := f.Stat()
		_ = f.Close()
		if err != nil || stat.IsDir() {
			serveIndexHTML(w, r, staticFS)
			return
		}
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
		httpServer:  httpServer,
		router:      r,
		logger:      logger,
		cfg:         cfg,
		db:          db,
		libRepo:     libRepo,
		settingRepo: settingRepo,
		mediaRepo:   mediaRepo,
	}
}

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

func (s *Server) Run() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s.logger.Info("server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	// Start library refresh scheduler
	go s.runLibraryRefreshScheduler(context.Background())

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

// runLibraryRefreshScheduler periodically checks library schedules and triggers refreshes.
func (s *Server) runLibraryRefreshScheduler(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRefreshLibraries(ctx)
		}
	}
}

// checkAndRefreshLibraries reads refresh schedules from settings and triggers overdue refreshes.
func (s *Server) checkAndRefreshLibraries(ctx context.Context) {
	// List all libraries
	libraries, err := s.libRepo.List(ctx)
	if err != nil {
		s.logger.Error("scheduler: failed to list libraries", "error", err)
		return
	}

	for _, lib := range libraries {
		intervalKey := "library_refresh_interval_" + lib.ID
		lastKey := "library_last_refresh_" + lib.ID

		intervalStr, err := s.settingRepo.GetSetting(ctx, intervalKey)
		if err != nil {
			// No schedule configured for this library
			continue
		}

		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval <= 0 {
			continue
		}

		lastRefreshStr, _ := s.settingRepo.GetSetting(ctx, lastKey)
		lastRefreshUnix, _ := strconv.ParseInt(lastRefreshStr, 10, 64)

		now := time.Now().Unix()
		if now-lastRefreshUnix < int64(interval) {
			continue // Not yet due
		}

		s.logger.Info("scheduler: triggering library refresh",
			"library_id", lib.ID,
			"name", lib.Name,
			"interval_seconds", interval,
		)

		// Trigger import
		imp := service.NewImporter(
			service.NewLocalImportFS(),
			"local",
			s.db,
			s.mediaRepo,
			repository.NewImportJobRepository(s.db),
		)
		imp.SetLibraryID(lib.ID)
		if _, err := imp.ImportRequest(ctx, lib.SourcePath); err != nil {
			s.logger.Error("scheduler: import failed", "library_id", lib.ID, "error", err)
			continue
		}

		// Record last refresh time
		_ = s.settingRepo.SetSetting(ctx, lastKey, strconv.FormatInt(now, 10))
	}
}