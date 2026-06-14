// Package server wires up the HTTP server and static file serving.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
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
	httpServer       *http.Server
	router           *chi.Mux
	logger           *slog.Logger
	cfg              *config.Config
	db               *repository.DB
	libRepo          *repository.LibraryRepository
	settingRepo      *repository.SystemSettingRepository
	mediaRepo        *repository.MediaRepository
	refreshCoordinator *RefreshCoordinator
	importWG         sync.WaitGroup
	shutdownOnce     sync.Once
	shutdownCh       chan struct{}
}

func New(cfg *config.Config, logger *slog.Logger, db *repository.DB, version, gitCommit, buildTime, goVer string) *Server {
	r := chi.NewRouter()
	r.Use(middleware.Logger(logger))
	r.Use(corsMiddleware)
	r.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)

	signer := presign.NewSigner(cfg.Auth.JWTSecret, 3600)
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))

	providerRepo := repository.NewProviderRepository(db)
	// Ensure local provider exists in DB (defense in addition to migration)
	if err := providerRepo.EnsureLocalProvider(context.Background()); err != nil {
		logger.Warn("failed to ensure local provider", "err", err)
	}
	if records, err := providerRepo.ListEnabled(context.Background()); err != nil {
		logger.Warn("failed to load providers from database", "err", err)
	} else {
		for _, rec := range records {
			// Skip 'local' — already registered in-memory above
			if rec.ID == "local" {
				continue
			}
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

	refreshCoordinator := NewRefreshCoordinator()

	healthHandler := handler.NewHealthHandler(version, gitCommit, buildTime, goVer)
	mediaHandler := handler.NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, libRepo, statusRepo, logger, refreshCoordinator)
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

	// Observability endpoints (no auth, no SPA fallback)
	diagHandler := handler.NewDiagHandler(db, FrontendAssetHash(web.Dist))
	r.Get("/healthz", diagHandler.Healthz)
	r.Get("/readyz", diagHandler.Readyz)
	r.Get("/version", diagHandler.Version)

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
		r.Get("/{id}/logo", mediaHandler.ServeLogo)
	})

	// Static files — go:embed dist embeds the dist/ directory at root.
	// So the FS root already IS dist/. No fs.Sub needed.
	// To open "assets/foo.js" we must use "dist/assets/foo.js".
	r.Get("/*", staticFileHandler(logger, web.Dist))
	r.Head("/*", staticFileHandler(logger, web.Dist))

	httpServer := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpServer:         httpServer,
		router:             r,
		logger:             logger,
		cfg:                cfg,
		db:                 db,
		libRepo:            libRepo,
		settingRepo:        settingRepo,
		mediaRepo:          mediaRepo,
		refreshCoordinator: refreshCoordinator,
		shutdownCh:         make(chan struct{}),
	}
}

// staticFileHandler returns an http.Handler that serves static files from
// the given FS. It handles brotli/gzip pre-compression, HEAD requests, and
// SPA index fallback. The FS root must be the dist/ directory (i.e., paths
// are resolved as "dist/assets/foo.js" for request "/assets/foo.js").
// Pass web.Dist for production use.
func staticFileHandler(logger *slog.Logger, fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "/" {
			name = "index.html"
		}

		logger.Info("STATIC REQUEST", "raw", r.URL.Path, "cleaned", name, "method", r.Method, "accept_encoding", r.Header.Get("Accept-Encoding"))

		// Try brotli first
		acceptEncoding := r.Header.Get("Accept-Encoding")
		if strings.Contains(acceptEncoding, "br") {
			brName := "dist/" + name + ".br"
			logger.Info("OPEN TRY", "file", brName)
			if data, err := fs.ReadFile(fsys, brName); err == nil {
				logger.Info("OPEN OK", "file", brName, "len", len(data))
				w.Header().Set("Content-Encoding", "br")
				w.Header().Set("Content-Type", detectContentType(name))
				w.Header().Set("Vary", "Accept-Encoding")
				setCacheHeader(w, name)
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			} else {
				logger.Info("OPEN FAIL", "file", brName, "err", err)
			}
		}

		// Try gzip
		if strings.Contains(acceptEncoding, "gzip") {
			gzName := "dist/" + name + ".gz"
			logger.Info("OPEN TRY", "file", gzName)
			if data, err := fs.ReadFile(fsys, gzName); err == nil {
				logger.Info("OPEN OK", "file", gzName, "len", len(data))
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Type", detectContentType(name))
				w.Header().Set("Vary", "Accept-Encoding")
				setCacheHeader(w, name)
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			} else {
				logger.Info("OPEN FAIL", "file", gzName, "err", err)
			}
		}

		// Serve uncompressed — read from FS and serve directly
		realName := "dist/" + name
		logger.Info("OPEN TRY", "file", realName)
		data, err := fs.ReadFile(fsys, realName)
		if err != nil {
			logger.Info("OPEN FAIL", "file", realName, "err", err)
			// For hashed assets under assets/, return 404 instead of SPA fallback.
			// This prevents CSS preload failures from receiving HTML.
			if isImmutableAsset(name) {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			serveIndexHTML(w, r, fsys)
			return
		}
		logger.Info("OPEN OK", "file", realName, "len", len(data))

		w.Header().Set("Content-Type", detectContentType(name))
		setCacheHeader(w, name)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func serveIndexHTML(w http.ResponseWriter, _ *http.Request, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "dist/index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func setCacheHeader(w http.ResponseWriter, name string) {
	if isImmutableAsset(name) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// detectContentType returns the MIME type based on the original file name.
// Important: use the original name (e.g. "assets/foo.css"), NOT the
// compressed path (e.g. "assets/foo.css.br") which would give wrong MIME.
func detectContentType(name string) string {
	ext := ""
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		ext = strings.ToLower(name[idx:])
	}
	switch ext {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".webmanifest":
		return "application/json; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

// isImmutableAsset returns true only for versioned assets under assets/.
// index.html, favicon.ico, robots.txt, etc. must NOT be immutable.
func isImmutableAsset(name string) bool {
	if !strings.HasPrefix(name, "assets/") {
		return false
	}
	// Double check: never immutable for these names
	if name == "index.html" || strings.HasSuffix(name, "/index.html") {
		return false
	}
	return true
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

	// Initiate shutdown sequence (idempotent)
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("server shutdown error", "error", err)
	}

	// Wait for in-flight imports to complete (with timeout)
	s.waitForImports(10 * time.Second)

	// Close database
	if err := s.db.Close(); err != nil {
		s.logger.Error("database close error", "error", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}

// waitForImports waits for in-flight imports to complete with a timeout.
func (s *Server) waitForImports(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.importWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("all in-flight imports completed")
	case <-time.After(timeout):
		s.logger.Warn("timeout waiting for in-flight imports to complete")
	}
}

// runLibraryRefreshScheduler periodically checks library schedules and triggers refreshes.
func (s *Server) runLibraryRefreshScheduler(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdownCh:
			s.logger.Info("scheduler: shutdown signal received, stopping")
			return
		case <-ticker.C:
			s.checkAndRefreshLibraries(ctx)
		}
	}
}

// checkAndRefreshLibraries reads refresh schedules from settings and triggers overdue refreshes.
func (s *Server) checkAndRefreshLibraries(ctx context.Context) {
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
			continue
		}

		// Try to acquire the refresh lock for this library
		if !s.refreshCoordinator.TryStart(lib.ID) {
			s.logger.Info("scheduler: refresh already running for library, skipping", "library_id", lib.ID, "name", lib.Name)
			continue
		}

		s.logger.Info("scheduler: triggering library refresh",
			"library_id", lib.ID,
			"name", lib.Name,
			"interval_seconds", interval,
		)

		// Track this import for shutdown handling
		s.importWG.Add(1)
		go func(libraryID, sourcePath string, intervalSec int) {
			defer s.importWG.Done()
			defer s.refreshCoordinator.Finish(libraryID)

			imp := service.NewImporter(
				service.NewLocalImportFS(),
				"local",
				s.db,
				s.mediaRepo,
				repository.NewImportJobRepository(s.db),
			)
			imp.SetLibraryID(libraryID)
			if _, err := imp.ImportRequest(ctx, sourcePath); err != nil {
				s.logger.Error("scheduler: import failed", "library_id", libraryID, "error", err)
				return
			}

			_ = s.settingRepo.SetSetting(ctx, "library_last_refresh_"+libraryID, strconv.FormatInt(time.Now().Unix(), 10))
		}(lib.ID, lib.SourcePath, interval)
	}
}

// corsMiddleware handles CORS preflight (OPTIONS) requests and adds
// Access-Control-Allow-* headers to all responses.
// It must be registered before ErrorHandler so that OPTIONS requests
// are intercepted before route matching returns 405.
func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:5173":  true,
		"http://127.0.0.1:5173": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Always set Vary headers to prevent caching issues.
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")

		// If origin is not in the allowed list, skip CORS headers
		// but still continue the request chain.
		if !allowedOrigins[origin] {
			next.ServeHTTP(w, r)
			return
		}

		// Set CORS headers for allowed origin.
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight OPTIONS request.
		if r.Method == http.MethodOptions {
			slog.Debug("CORS preflight",
				"method", r.Method,
				"path", r.URL.Path,
				"origin", origin,
				"access-control-request-method", r.Header.Get("Access-Control-Request-Method"),
				"access-control-request-headers", r.Header.Get("Access-Control-Request-Headers"),
			)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
