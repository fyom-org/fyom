// Package errors defines fyom-specific error types and helpers.
//
// This file defines the stable, machine-readable error_code taxonomy emitted
// by every API error response. The frontend uses error_code (NOT the
// human-readable message) as the i18n lookup key, so:
//
//   - Renaming a code is a breaking change for clients.
//   - Adding new codes is always safe.
//   - The English `message` field is a fallback for clients that do not yet
//     ship a translation for the code.
//
// Code naming conventions:
//   - snake_case
//   - verb_failed_past_tense for runtime failures (e.g. failed_to_create_provider)
//   - noun_required / noun_too_long / noun_invalid for validation errors
//   - noun_not_found for lookup misses
//   - Use `internal_error` as the catch-all for unexpected 500s (never leak
//     the underlying Go error to the client).
package errors

// Code is a stable, machine-readable error identifier.
//
// It is emitted as the `error_code` field of the standard API response
// envelope (see pkg/response.Response). The empty string means "no code
// available" (legacy responses, success responses, or unclassified errors).
type Code string

const (
	// ----- Generic / catch-all -----

	// CodeInternal is the catch-all for unexpected 5xx errors.
	// The underlying error is logged but never sent to the client.
	CodeInternal Code = "internal_error"
	// CodeValidation is a generic 400 for malformed/missing request bodies
	// when no more specific code applies.
	CodeValidation Code = "validation_error"
	// CodeInvalidJSON is returned when the request body is not valid JSON.
	CodeInvalidJSON Code = "invalid_json"
	// CodeBadRequest is a generic 400 for malformed requests.
	CodeBadRequest Code = "bad_request"
	// CodeConflict is a generic 409 for concurrent-modification races.
	CodeConflict Code = "conflict"

	// ----- Auth (401 / 403) -----

	// CodeUnauthorized is returned when no valid authentication is present.
	CodeUnauthorized Code = "unauthorized"
	// CodeForbidden is returned when the authenticated user lacks permission.
	CodeForbidden Code = "forbidden"
	// CodeInvalidCredentials is returned when login credentials don't match.
	CodeInvalidCredentials Code = "invalid_credentials"
	// CodeRegistrationDisabled is returned when self-registration is off.
	CodeRegistrationDisabled Code = "registration_disabled"
	// CodeAdminRoleRequired is returned when an admin-only endpoint is hit
	// by a non-admin user.
	CodeAdminRoleRequired Code = "admin_role_required"
	// CodeLocalhostOnly is returned when an endpoint that must be invoked
	// from localhost is hit from a remote address.
	CodeLocalhostOnly Code = "localhost_only"
	// CodeMissingAuthHeader is returned when the Authorization header is absent.
	CodeMissingAuthHeader Code = "missing_auth_header"
	// CodeInvalidAuthHeader is returned when the Authorization header is
	// malformed (e.g. not "Bearer <token>").
	CodeInvalidAuthHeader Code = "invalid_auth_header"
	// CodeTokenMissingSubject is returned when a JWT lacks the subject claim.
	CodeTokenMissingSubject Code = "token_missing_subject"
	// CodeFailedToIssueToken is returned when JWT signing fails unexpectedly.
	CodeFailedToIssueToken Code = "failed_to_issue_token"

	// ----- Bootstrap / first-run -----

	// CodeNoBootstrapToken is returned when a bootstrap token is requested
	// but none is available.
	CodeNoBootstrapToken Code = "no_bootstrap_token"
	// CodeNoBootstrapSession is returned when a bootstrap session is requested
	// but none is active.
	CodeNoBootstrapSession Code = "no_bootstrap_session"
	// CodeAlreadyInitialized is returned when a first-run setup is attempted
	// on an already-provisioned server.
	CodeAlreadyInitialized Code = "already_initialized"

	// ----- Resource lookup (404) -----

	// CodeNotFound is a generic 404.
	CodeNotFound Code = "not_found"
	// CodeResourceNotFound is a 404 returned by handlers that wrap AppError.
	CodeResourceNotFound Code = "resource_not_found"
	// CodeLibraryNotFound is returned when a referenced library does not exist.
	CodeLibraryNotFound Code = "library_not_found"
	// CodeProviderNotFound is returned when a referenced provider does not exist.
	CodeProviderNotFound Code = "provider_not_found"
	// CodeMissingID is returned when an ID path/query parameter is absent.
	CodeMissingID Code = "missing_id"

	// ----- Validation specifics (400) -----

	// CodeNameRequired is returned when a required `name` field is empty.
	CodeNameRequired Code = "name_required"
	// CodeDisplayNameRequired is returned when a required `display_name` is empty.
	CodeDisplayNameRequired Code = "display_name_required"
	// CodeDisplayNameTooLong is returned when `display_name` exceeds 128 chars.
	CodeDisplayNameTooLong Code = "display_name_too_long"
	// CodeIDRequired is returned when a required `id` field is empty.
	CodeIDRequired Code = "id_required"
	// CodeIDTooLong is returned when an `id` exceeds 64 chars.
	CodeIDTooLong Code = "id_too_long"
	// CodeIDHasSpaces is returned when an `id` contains whitespace.
	CodeIDHasSpaces Code = "id_has_spaces"
	// CodeIDLocalReserved is returned when a user tries to create a provider
	// with the reserved id "local".
	CodeIDLocalReserved Code = "id_local_reserved"
	// CodeNewPasswordRequired is returned when change-password is called
	// without `new_password`.
	CodeNewPasswordRequired Code = "new_password_required"
	// CodeOldPasswordRequired is returned when change-password is called
	// without `old_password`.
	CodeOldPasswordRequired Code = "old_password_required"
	// CodeLibraryIDRequired is returned when a request lacks `library_id`.
	CodeLibraryIDRequired Code = "library_id_required"
	// CodeUserIDAndLibraryIDRequired is returned when a permission change
	// lacks either `user_id` or `library_id`.
	CodeUserIDAndLibraryIDRequired Code = "user_id_and_library_id_required"
	// CodeConfigInvalidJSON is returned when a provider config is not valid JSON.
	CodeConfigInvalidJSON Code = "config_invalid_json"
	// CodeTypeInvalid is returned when a `type` field is not one of the allowed values.
	CodeTypeInvalid Code = "type_invalid"
	// CodeUnsupportedLocale is returned when a preferred_language is not in
	// the supported locales list.
	CodeUnsupportedLocale Code = "unsupported_locale"
	// CodeInvalidStatus is returned when a media status is not in the
	// allowed enum.
	CodeInvalidStatus Code = "invalid_status"
	// CodeInvalidProgress is returned when watch progress is out of range.
	CodeInvalidProgress Code = "invalid_progress"
	// CodeInvalidMode is returned when a delete `mode` is not 'cascade'.
	CodeInvalidMode Code = "invalid_mode"
	// CodeUnknownSetting is returned when an admin tries to update a
	// setting key that does not exist.
	CodeUnknownSetting Code = "unknown_setting"
	// CodeImportFromProviderTypeUnsupported is returned when an import is
	// attempted from a provider type that does not support import.
	CodeImportFromProviderTypeUnsupported Code = "import_from_provider_type_unsupported"

	// ----- Provider ops (5xx) -----

	// CodeFailedToCreateProvider indicates that provider creation failed.
	CodeFailedToCreateProvider Code = "failed_to_create_provider"
	// CodeFailedToUpdateProvider indicates that provider update failed.
	CodeFailedToUpdateProvider Code = "failed_to_update_provider"
	// CodeFailedToDeleteProvider indicates that provider deletion failed.
	CodeFailedToDeleteProvider Code = "failed_to_delete_provider"
	// CodeFailedToLoadProviderConfig indicates that provider config loading failed.
	CodeFailedToLoadProviderConfig Code = "failed_to_load_provider_config"
	// CodeFailedToCreateS3Client indicates that S3 client creation failed.
	CodeFailedToCreateS3Client Code = "failed_to_create_s3_client"

	// ----- Library ops -----

	// CodeRefreshAlreadyInProgress is returned when a library refresh is
	// requested while another is already running.
	CodeRefreshAlreadyInProgress Code = "refresh_already_in_progress"
	// CodeOrphanModeDeleteItemsFirst is returned when a library is deleted
	// in `orphan` mode while it still contains items.
	CodeOrphanModeDeleteItemsFirst Code = "orphan_mode_delete_items_first"

	// ----- Media ops -----

	// CodeMediaItemNotShow is returned when a show-only operation is invoked
	// on a non-show media item.
	CodeMediaItemNotShow Code = "media_item_not_show"
	// CodeCannotUpdateProgressForShow is returned when watch progress is
	// attempted on a show (must be on an episode).
	CodeCannotUpdateProgressForShow Code = "cannot_update_progress_for_show"
)

// defaultMessages maps each Code to its canonical English fallback message.
//
// These are intentionally short, human-readable strings that mirror the
// pre-Phase-3 hardcoded messages, preserving backward compatibility for
// any client that reads the `message` field instead of `error_code`.
//
// Keep this map in sync with web/src/locales/en.json -> api_error.*
// and web/src/locales/zh.json -> api_error.*.
var defaultMessages = map[Code]string{
	CodeInternal:    "internal server error",
	CodeValidation:  "validation error",
	CodeInvalidJSON: "invalid JSON",
	CodeBadRequest:  "bad request",
	CodeConflict:    "conflict",

	CodeUnauthorized:         "unauthorized",
	CodeForbidden:            "forbidden",
	CodeInvalidCredentials:   "invalid credentials",
	CodeRegistrationDisabled: "registration is disabled",
	CodeAdminRoleRequired:    "admin role required",
	CodeLocalhostOnly:        "forbidden: localhost only",
	CodeMissingAuthHeader:    "missing authorization header",
	CodeInvalidAuthHeader:    "invalid authorization header format",
	CodeTokenMissingSubject:  "token missing subject claim",
	CodeFailedToIssueToken:   "failed to issue token",

	CodeNoBootstrapToken:   "no bootstrap token available",
	CodeNoBootstrapSession: "no bootstrap session available",
	CodeAlreadyInitialized: "already initialized",

	CodeNotFound:         "not found",
	CodeResourceNotFound: "resource not found",
	CodeLibraryNotFound:  "library not found",
	CodeProviderNotFound: "provider not found",
	CodeMissingID:        "missing id",

	CodeNameRequired:                      "name is required",
	CodeDisplayNameRequired:               "display_name is required",
	CodeDisplayNameTooLong:                "display_name must be at most 128 characters",
	CodeIDRequired:                        "id is required",
	CodeIDTooLong:                         "id must be at most 64 characters",
	CodeIDHasSpaces:                       "id must not contain spaces",
	CodeIDLocalReserved:                   "id \"local\" is reserved for the built-in LocalProvider",
	CodeNewPasswordRequired:               "new_password is required",
	CodeOldPasswordRequired:               "old_password is required",
	CodeLibraryIDRequired:                 "library_id is required",
	CodeUserIDAndLibraryIDRequired:        "user_id and library_id are required",
	CodeConfigInvalidJSON:                 "config must be valid JSON",
	CodeTypeInvalid:                       "type must be one of: s3, remote_fyom",
	CodeUnsupportedLocale:                 "unsupported locale",
	CodeInvalidStatus:                     "invalid status",
	CodeInvalidProgress:                   "invalid progress",
	CodeInvalidMode:                       "invalid mode: use 'cascade'",
	CodeUnknownSetting:                    "unknown setting",
	CodeImportFromProviderTypeUnsupported: "import from this provider type is not supported yet",

	CodeFailedToCreateProvider:     "failed to create provider",
	CodeFailedToUpdateProvider:     "failed to update provider",
	CodeFailedToDeleteProvider:     "failed to delete provider",
	CodeFailedToLoadProviderConfig: "failed to load provider config",
	CodeFailedToCreateS3Client:     "failed to create S3 client",

	CodeRefreshAlreadyInProgress:   "refresh already in progress for this library",
	CodeOrphanModeDeleteItemsFirst: "orphan mode: delete items first, then delete the empty library",

	CodeMediaItemNotShow:            "media item is not a show",
	CodeCannotUpdateProgressForShow: "cannot update progress for show",
}

// DefaultMessage returns the canonical English fallback message for a code.
//
// If the code is unknown or empty, returns "internal server error" — never
// an empty string, so callers can safely use this as a last-resort message.
func (c Code) DefaultMessage() string {
	if msg, ok := defaultMessages[c]; ok && msg != "" {
		return msg
	}
	return "internal server error"
}

// String implements fmt.Stringer.
func (c Code) String() string { return string(c) }

// IsValid reports whether c is a registered error code.
func (c Code) IsValid() bool {
	_, ok := defaultMessages[c]
	return ok
}
