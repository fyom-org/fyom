package handler

import (
	"encoding/json"
	"net/http"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/locale"
	"github.com/fyom/fyom/pkg/response"
)

// SystemHandler handles system-level endpoints.
type SystemHandler struct {
	settingRepo *repository.SystemSettingRepository
	authService *service.AuthService
}

// NewSystemHandler creates a new SystemHandler.
func NewSystemHandler(settingRepo *repository.SystemSettingRepository, authService *service.AuthService) *SystemHandler {
	return &SystemHandler{
		settingRepo: settingRepo,
		authService: authService,
	}
}

// StatusResponse holds the system status.
type StatusResponse struct {
	Initialized       bool     `json:"initialized"`
	AllowRegistration bool     `json:"allow_registration"`
	DefaultLocale     string   `json:"default_locale"`
	SupportedLocales  []string `json:"supported_locales"`
}

// Status returns whether the system has been initialized and if registration is open.
//
// Also returns i18n configuration:
//   - default_locale: the admin-configured system default locale (from
//     system_settings.default_locale). Falls back to pkg/locale.DefaultLocale
//     ("en") if unset.
//   - supported_locales: the list of locale codes the frontend can render.
//
// The frontend uses these to populate the admin Settings default-locale
// dropdown and to validate user preferences.
func (h *SystemHandler) Status(w http.ResponseWriter, r *http.Request) {
	initialized, _ := h.settingRepo.GetSetting(r.Context(), "initialized")
	allowReg, _ := h.settingRepo.GetSetting(r.Context(), "allow_registration")
	defaultLocale, _ := h.settingRepo.GetSetting(r.Context(), "default_locale")

	if defaultLocale == "" || !locale.IsValid(defaultLocale) {
		defaultLocale = locale.DefaultLocale
	}

	response.Success(w, StatusResponse{
		Initialized:       initialized == "true",
		AllowRegistration: allowReg == "true",
		DefaultLocale:     defaultLocale,
		SupportedLocales:  locale.SupportedLocales,
	})
}

// InitializeRequest holds the setup wizard form data.
type InitializeRequest struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	AllowRegistration bool   `json:"allow_registration"`
}

// Initialize handles first-run setup: creates the admin user and marks system as initialized.
func (h *SystemHandler) Initialize(w http.ResponseWriter, r *http.Request) {
	// Check if already initialized
	initialized, err := h.settingRepo.GetSetting(r.Context(), "initialized")
	if err != nil {
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}
	if initialized == "true" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeAlreadyInitialized, "")
		return
	}

	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeValidation, "")
		return
	}

	// Create the admin user (Register will auto-assign admin since count == 0)
	user, err := h.authService.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			response.AppError(w, appErr)
			return
		}
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	// Mark as initialized and set registration preference
	_ = h.settingRepo.SetSetting(r.Context(), "initialized", "true")
	regVal := "false"
	if req.AllowRegistration {
		regVal = "true"
	}
	_ = h.settingRepo.SetSetting(r.Context(), "allow_registration", regVal)

	response.Success(w, map[string]interface{}{
		"username": user.Username,
		"role":     user.Role,
	})
}
