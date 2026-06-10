package provider

import (
	"encoding/json"
	"fmt"
)

// S3Config holds the configuration for an S3-compatible storage provider.
// It is deserialized from the raw JSON stored in the providers table.
type S3Config struct {
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	// Endpoint is optional. Empty means AWS S3.
	// Set for compatible services:
	//   Wasabi:  https://s3.wasabisys.com
	//   B2:      https://s3.us-west-000.backblazeb2.com
	//   MinIO:   http://minio:9000
	Endpoint string `json:"endpoint"`
	// PathPrefix is the optional S3 key prefix used by the importer to scope
	// which objects belong to this provider. It is NOT prepended by the
	// provider at URL-generation time — file_path already contains the full key.
	PathPrefix string `json:"path_prefix"`
	// CDNBaseURL is optional. When non-empty, the scheme+host of generated
	// presigned URLs is replaced with this value.
	// Example: "https://cdn.example.com"
	// The query string (signature params) is preserved unchanged.
	CDNBaseURL string `json:"cdn_base_url"`
}

// parseS3Config deserializes and validates an S3Config from raw JSON.
// Returns an error if required fields are missing or the JSON is invalid.
func parseS3Config(raw string) (S3Config, error) {
	var cfg S3Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid S3 config JSON: %w", err)
	}
	if cfg.Bucket == "" {
		return cfg, fmt.Errorf("S3 config: bucket is required")
	}
	if cfg.Region == "" {
		return cfg, fmt.Errorf("S3 config: region is required")
	}
	if cfg.AccessKeyID == "" {
		return cfg, fmt.Errorf("S3 config: access_key_id is required")
	}
	if cfg.SecretAccessKey == "" {
		return cfg, fmt.Errorf("S3 config: secret_access_key is required")
	}
	return cfg, nil
}
