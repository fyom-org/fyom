package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
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
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
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
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidJSON, "")
		return
	}

	// Validate ID.
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeIDRequired, "")
		return
	}
	if strings.Contains(req.ID, " ") {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeIDHasSpaces, "")
		return
	}
	if len(req.ID) > 64 {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeIDTooLong, "")
		return
	}
	if req.ID == "local" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeIDLocalReserved, "")
		return
	}

	// Validate type.
	if !allowedProviderTypes[req.Type] {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeTypeInvalid, "")
		return
	}

	// Validate display name.
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeDisplayNameRequired, "")
		return
	}
	if len(req.DisplayName) > 128 {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeDisplayNameTooLong, "")
		return
	}

	// Validate config is valid JSON.
	if req.Config != nil {
		var raw json.RawMessage
		if err := json.Unmarshal(req.Config, &raw); err != nil {
			response.ErrorCode(w, http.StatusBadRequest, errors.CodeConfigInvalidJSON, "")
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
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeFailedToCreateProvider, "")
		return
	}

	w.WriteHeader(http.StatusCreated)
	response.Success(w, record)
}

// UpdateProvider updates display_name, config, and enabled for an existing provider.
func (h *AdminProviderHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMissingID, "")
		return
	}

	var req struct {
		DisplayName string          `json:"display_name"`
		Config      json.RawMessage `json:"config"`
		Enabled     bool            `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidJSON, "")
		return
	}

	// Validate display name.
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeDisplayNameRequired, "")
		return
	}
	if len(req.DisplayName) > 128 {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeDisplayNameTooLong, "")
		return
	}

	// Validate config is valid JSON.
	if req.Config != nil {
		var raw json.RawMessage
		if err := json.Unmarshal(req.Config, &raw); err != nil {
			response.ErrorCode(w, http.StatusBadRequest, errors.CodeConfigInvalidJSON, "")
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
			response.ErrorCode(w, http.StatusNotFound, errors.CodeProviderNotFound, "")
			return
		}
		h.logger.Error("failed to update provider", "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeFailedToUpdateProvider, "")
		return
	}

	// Return the updated record.
	updated, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}
	response.Success(w, updated)
}

// DeleteProvider removes a provider record.
func (h *AdminProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMissingID, "")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "provider not found") {
			response.ErrorCode(w, http.StatusNotFound, errors.CodeProviderNotFound, "")
			return
		}
		if strings.Contains(errStr, "still reference it") {
			response.ErrorCode(w, http.StatusConflict, errors.CodeConflict, errStr)
			return
		}
		h.logger.Error("failed to delete provider", "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeFailedToDeleteProvider, "")
		return
	}

	response.NoContent(w)
}
