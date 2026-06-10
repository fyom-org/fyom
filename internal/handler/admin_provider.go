package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

// AdminProviderHandler handles CRUD for provider configuration.
// All endpoints require admin role — register behind RequireAdmin middleware.
type AdminProviderHandler struct {
	repo   *repository.ProviderRepository
	logger *slog.Logger
}

// NewAdminProviderHandler creates a new AdminProviderHandler.
func NewAdminProviderHandler(repo *repository.ProviderRepository, logger *slog.Logger) *AdminProviderHandler {
	return &AdminProviderHandler{repo: repo, logger: logger}
}

// allowedProviderTypes lists the provider types accepted by the POST handler.
// "local" is intentionally excluded — it is hardcoded at startup.
var allowedProviderTypes = map[string]bool{
	"s3":          true,
	"remote_fyom": true,
}

// ListProviders returns all providers (enabled and disabled).
func (h *AdminProviderHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.repo.List(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if providers == nil {
		providers = []model.ProviderRecord{}
	}
	response.Success(w, providers)
}

// CreateProvider creates a new provider record.
func (h *AdminProviderHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		DisplayName string          `json:"display_name"`
		Config      json.RawMessage `json:"config"`
		Enabled     bool            `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}

	// Validate ID.
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		response.Error(w, 400, "id is required")
		return
	}
	if strings.Contains(req.ID, " ") {
		response.Error(w, 400, "id must not contain spaces")
		return
	}
	if len(req.ID) > 64 {
		response.Error(w, 400, "id must be at most 64 characters")
		return
	}
	if req.ID == "local" {
		response.Error(w, 400, "id \"local\" is reserved for the built-in LocalProvider")
		return
	}

	// Validate type.
	if !allowedProviderTypes[req.Type] {
		response.Error(w, 400, "type must be one of: s3, remote_fyom")
		return
	}

	// Validate display name.
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		response.Error(w, 400, "display_name is required")
		return
	}
	if len(req.DisplayName) > 128 {
		response.Error(w, 400, "display_name must be at most 128 characters")
		return
	}

	// Validate config is valid JSON.
	if req.Config != nil {
		var raw json.RawMessage
		if err := json.Unmarshal(req.Config, &raw); err != nil {
			response.Error(w, 400, "config must be valid JSON")
			return
		}
	}

	// Default config to "{}" if not provided.
	configStr := string(req.Config)
	if configStr == "" {
		configStr = "{}"
	}

	record := &model.ProviderRecord{
		ID:          req.ID,
		Type:        req.Type,
		DisplayName: req.DisplayName,
		Config:      configStr,
		Enabled:     req.Enabled,
	}

	if err := h.repo.Create(r.Context(), record); err != nil {
		h.logger.Error("failed to create provider", "err", err)
		response.Error(w, 500, "failed to create provider")
		return
	}

	w.WriteHeader(http.StatusCreated)
	response.Success(w, record)
}

// UpdateProvider updates display_name, config, and enabled for an existing provider.
func (h *AdminProviderHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.Error(w, 400, "missing id")
		return
	}

	var req struct {
		DisplayName string          `json:"display_name"`
		Config      json.RawMessage `json:"config"`
		Enabled     bool            `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}

	// Validate display name.
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		response.Error(w, 400, "display_name is required")
		return
	}
	if len(req.DisplayName) > 128 {
		response.Error(w, 400, "display_name must be at most 128 characters")
		return
	}

	// Validate config is valid JSON.
	if req.Config != nil {
		var raw json.RawMessage
		if err := json.Unmarshal(req.Config, &raw); err != nil {
			response.Error(w, 400, "config must be valid JSON")
			return
		}
	}

	configStr := string(req.Config)
	if configStr == "" {
		configStr = "{}"
	}

	record := &model.ProviderRecord{
		ID:          id,
		DisplayName: req.DisplayName,
		Config:      configStr,
		Enabled:     req.Enabled,
	}

	if err := h.repo.Update(r.Context(), record); err != nil {
		if strings.Contains(err.Error(), "provider not found") {
			response.Error(w, 404, "provider not found")
			return
		}
		h.logger.Error("failed to update provider", "err", err)
		response.Error(w, 500, "failed to update provider")
		return
	}

	// Return the updated record.
	updated, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	response.Success(w, updated)
}

// DeleteProvider removes a provider record.
func (h *AdminProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.Error(w, 400, "missing id")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "provider not found") {
			response.Error(w, 404, "provider not found")
			return
		}
		if strings.Contains(errStr, "still reference it") {
			response.Error(w, 409, errStr)
			return
		}
		h.logger.Error("failed to delete provider", "err", err)
		response.Error(w, 500, "failed to delete provider")
		return
	}

	response.NoContent(w)
}
