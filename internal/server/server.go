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

	web "github.com/fyom/fyom/frontend"
	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/handler"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
)

// Server wraps the HTTP server and all its dependencies for graceful startup and shutdown.
type Server struct {
	httpServer         *http.Server
	router             *chi.Mux
	logger             *slog.Logger
	cfg                *config.Config
	db                 *repository.DB
	libRepo            *repository.LibraryRepository
	settingRepo        *repository.SystemSettingRepository
	mediaRepo          *repository.MediaRepository
	refreshCoordinator *RefreshCoordinator
	bootstrapSvc       *service.BootstrapService
	importWG           sync.WaitGroup
	shutdownOnce       sync.Once
	shutdownCh         chan struct{}
}

// Router returns the underlying Chi router for use in-process (e.g. desktop mode).
func (s *Server) Router() http.Handler {
	return s.router
}

// New creates a new Server with the given configuration and dependencies.
// It sets up the HTTP router, registers all routes, and initializes handlers.
func New(
	cfg *config.Config,
	logger *slog.Logger,
	db *repository.DB,
	version string,
	gitCommit string,
	buildTime string,
	goVer string,
	runMode service.BootstrapMode,
) *Server {
	_ = runMode

	r := chi.NewRouter()
	r.Use(middleware.Logger(logger))
	r.Use(corsMiddleware)
	r.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	statusRepo := repository.NewUserMediaStatusRepository(db)

	signer := presign.NewSigner(cfg.Auth.JWTSecret, 3600)

	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))

	if err := providerRepo.EnsureLocalProvider(context.Background()); err != nil {
		logger.Warn("failed to ensure local provider", "err", err)
	}

	if records, err := providerRepo.ListEnabled(context.Background()); err != nil {
		logger.Warn("failed to load providers from database", "err", err)
	} else {
		for _, rec := range records {
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

	refreshCoordinator := NewRefreshCoordinator()

	healthHandler := handler.NewHealthHandler(version, gitCommit, buildTime, goVer)
	mediaHandler := handler.NewMediaHandler(
		reg,
		db,
		mediaRepo,
		jobRepo,
		providerRepo,
		libRepo,
		statusRepo,
		logger,
		refreshCoordinator,
	)
	authHandler := handler.NewAuthHandler(
		userRepo,
		libPermRepo,
		settingRepo,
		cfg.Auth.JWTSecret,
		cfg.Auth.TokenExpiry,
	)
	systemHandler := handler.NewSystemHandler(settingRepo, authHandler.GetAuthService())
	adminProviderHandler := handler.NewAdminProviderHandler(providerRepo, logger)
	adminHandler := handler.NewAdminHandler(adminRepo, mediaRepo, settingRepo, libPermRepo, db)
	adminLibHandler := handler.NewAdminLibraryHandler(libRepo, providerRepo, libPermRepo)
	bootstrapSvc := service.NewBootstrapService(authHandler.GetAuthService(), userRepo, settingRepo)
	diagHandler := handler.NewDiagHandler(db, FrontendAssetHash(web.Dist))

	registerPublicRoutes(r, healthHandler, systemHandler, authHandler, diagHandler)
	registerUserRoutes(
		r,
		cfg,
		userRepo,
		libPermRepo,
		mediaHandler,
		authHandler,
	)
	registerAdminRoutes(
		r,
		cfg,
		userRepo,
		adminHandler,
		adminLibHandler,
		adminProviderHandler,
	)
	registerPresignedMediaRoutes(r, signer, mediaHandler)
	registerStaticRoutes(r, logger)

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
		bootstrapSvc:       bootstrapSvc,
		shutdownCh:         make(chan struct{}),
	}
}

func registerPublicRoutes(
	r chi.Router,
	healthHandler *handler.HealthHandler,
	systemHandler *handler.SystemHandler,
	authHandler *handler.AuthHandler,
	diagHandler *handler.DiagHandler,
) {
	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)

	r.Get("/api/v1/system/status", systemHandler.Status)
	r.Post("/api/v1/system/initialize", systemHandler.Initialize)

	r.Post("/api/v1/auth/register", authHandler.Register)
	r.Post("/api/v1/auth/login", authHandler.Login)
	r.Get("/api/v1/auth/desktop-bootstrap", authHandler.DesktopBootstrap)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowLocalOnly)
		r.Get("/api/v1/internal/bootstrap-session", authHandler.InternalBootstrapSession)
	})

	r.Get("/healthz", diagHandler.Healthz)
	r.Get("/readyz", diagHandler.Readyz)
	r.Get("/version", diagHandler.Version)
}

func registerUserRoutes(
	r chi.Router,
	cfg *config.Config,
	userRepo *repository.UserRepository,
	libPermRepo *repository.LibraryPermissionRepository,
	mediaHandler *handler.MediaHandler,
	authHandler *handler.AuthHandler,
) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo(cfg.Auth.JWTSecret, userRepo))
		r.Use(middleware.ResolvePermissions(libPermRepo))

		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Put("/api/v1/auth/me/password", authHandler.ChangePassword)
		r.Put("/api/v1/auth/me/preferences", authHandler.UpdatePreferences)

		r.Get("/api/v1/libraries", mediaHandler.GetLibraries)

		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/continue", mediaHandler.GetContinueWatching)
		r.Get("/api/v1/library/by-status", mediaHandler.GetByStatus)
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library/{id}/episodes", mediaHandler.ListEpisodes)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)

		r.Put("/api/v1/media/{id}/progress", mediaHandler.UpdateProgress)
		r.Get("/api/v1/media/{id}/progress", mediaHandler.GetProgress)
		r.Put("/api/v1/media/{id}/status", mediaHandler.SetStatus)
		r.Get("/api/v1/media/{id}/status", mediaHandler.GetStatus)

		// Library import and destructive delete are admin-only operations.
		// They stay in this group to reuse resolved permissions and current-user
		// verification, but RequireAdmin is applied explicitly.
		r.With(middleware.RequireAdmin).Post("/api/v1/library/import", mediaHandler.Import)
		r.With(middleware.RequireAdmin).Delete("/api/v1/library/{id}", mediaHandler.Delete)
	})
}

func registerAdminRoutes(
	r chi.Router,
	cfg *config.Config,
	userRepo *repository.UserRepository,
	adminHandler *handler.AdminHandler,
	adminLibHandler *handler.AdminLibraryHandler,
	adminProviderHandler *handler.AdminProviderHandler,
) {
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo(cfg.Auth.JWTSecret, userRepo))
		r.Use(middleware.RequireAdmin)

		r.Get("/stats", adminHandler.GetStats)
		r.Get("/import-jobs", adminHandler.ListImportJobs)

		r.Get("/settings", adminHandler.GetSettings)
		r.Put("/settings", adminHandler.UpdateSettings)

		r.Get("/media", adminHandler.ListMedia)
		r.Delete("/media/{id}", adminHandler.DeleteMedia)

		r.Get("/media/missing", adminHandler.ListMissing)
		r.Delete("/media/missing", adminHandler.DeleteMissing)

		r.Get("/libraries", adminLibHandler.List)
		r.Post("/libraries", adminLibHandler.Create)
		r.Get("/libraries/{id}", adminLibHandler.Get)
		r.Put("/libraries/{id}", adminLibHandler.Update)
		r.Delete("/libraries/{id}", adminLibHandler.Delete)
		r.Delete("/libraries/{id}/items", adminLibHandler.DeleteLibraryWithItems)
		r.Post("/libraries/{id}/refresh", adminLibHandler.Refresh)
		r.Post("/libraries/{id}/check-missing", adminLibHandler.CheckMissing)

		r.Get("/permissions", adminHandler.ListPermissions)
		r.Put("/permissions", adminHandler.UpdatePermission)

		r.Get("/providers", adminProviderHandler.ListProviders)
		r.Post("/providers", adminProviderHandler.CreateProvider)
		r.Put("/providers/{id}", adminProviderHandler.UpdateProvider)
		r.Delete("/providers/{id}", adminProviderHandler.DeleteProvider)
	})
}

func registerPresignedMediaRoutes(
	r chi.Router,
	signer *presign.Signer,
	mediaHandler *handler.MediaHandler,
) {
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.RequireValidPresign(signer))

		// These endpoints are authorized by the presigned token.
		// If media handlers perform library permission checks, they must treat
		// valid presigned requests as already authorized or use a presign-aware
		// access path.
		r.Get("/{id}/poster", mediaHandler.Poster)
		r.Get("/{id}/backdrop", mediaHandler.ServeBackdrop)
		r.Get("/{id}/stream", mediaHandler.Stream)
		r.Get("/{id}/logo", mediaHandler.ServeLogo)
	})
}

func registerStaticRoutes(r chi.Router, logger *slog.Logger) {
	r.Get("/*", staticFileHandler(logger, web.Dist))
	r.Head("/*", staticFileHandler(logger, web.Dist))
}

// RunBootstrap executes the initial bootstrap if needed.
// It should be called once after the server is created but before it starts listening.
func (s *Server) RunBootstrap(ctx context.Context, mode service.BootstrapMode) (*service.BootstrapResult, error) {
	return s.bootstrapSvc.EnsureInitialBootstrap(ctx, mode)
}

func staticFileHandler(logger *slog.Logger, fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "/" {
			name = "index.html"
		}

		logger.Debug(
			"static request",
			"raw", r.URL.Path,
			"cleaned", name,
			"method", r.Method,
			"accept_encoding", r.Header.Get("Accept-Encoding"),
		)

		acceptEncoding := r.Header.Get("Accept-Encoding")

		if strings.Contains(acceptEncoding, "br") {
			brName := "dist/" + name + ".br"
			if data, err := fs.ReadFile(fsys, brName); err == nil {
				writeStaticBytes(w, r, name, "br", data)
				return
			}
		}

		if strings.Contains(acceptEncoding, "gzip") {
			gzName := "dist/" + name + ".gz"
			if data, err := fs.ReadFile(fsys, gzName); err == nil {
				writeStaticBytes(w, r, name, "gzip", data)
				return
			}
		}

		realName := "dist/" + name
		data, err := fs.ReadFile(fsys, realName)
		if err != nil {
			if isImmutableAsset(name) {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}

			serveIndexHTML(w, r, fsys)
			return
		}

		writeStaticBytes(w, r, name, "", data)
	}
}

func writeStaticBytes(w http.ResponseWriter, r *http.Request, name string, encoding string, data []byte) {
	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
		w.Header().Set("Vary", "Accept-Encoding")
	}

	w.Header().Set("Content-Type", detectContentType(name))
	setCacheHeader(w, name)

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
}

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

func isImmutableAsset(name string) bool {
	if !strings.HasPrefix(name, "assets/") {
		return false
	}

	if name == "index.html" || strings.HasSuffix(name, "/index.html") {
		return false
	}

	return true
}

// Run starts the HTTP server and blocks until shutdown.
// It handles graceful shutdown on SIGINT/SIGTERM, waiting for in-flight imports to complete.
func (s *Server) Run() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s.logger.Info("server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	go s.runLibraryRefreshScheduler(context.Background())

	<-quit
	s.logger.Info("server shutting down")

	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("server shutdown error", "error", err)
	}

	s.waitForImports(10 * time.Second)

	if err := s.db.Close(); err != nil {
		s.logger.Error("database close error", "error", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}

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

		if !s.refreshCoordinator.TryStart(lib.ID) {
			s.logger.Info(
				"scheduler: refresh already running for library, skipping",
				"library_id", lib.ID,
				"name", lib.Name,
			)
			continue
		}

		s.logger.Info(
			"scheduler: triggering library refresh",
			"library_id", lib.ID,
			"name", lib.Name,
			"interval_seconds", interval,
		)

		s.importWG.Add(1)

		go func(libraryID string, sourcePath string) {
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

			_ = s.settingRepo.SetSetting(
				ctx,
				"library_last_refresh_"+libraryID,
				strconv.FormatInt(time.Now().Unix(), 10),
			)
		}(lib.ID, lib.SourcePath)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:5173": true,
		"http://127.0.0.1:5173": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")

		if !allowedOrigins[origin] {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			slog.Debug(
				"CORS preflight",
				"method", r.Method,
				"path", r.URL.Path,
				"origin", origin,
				"access_control_request_method", r.Header.Get("Access-Control-Request-Method"),
				"access_control_request_headers", r.Header.Get("Access-Control-Request-Headers"),
			)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
