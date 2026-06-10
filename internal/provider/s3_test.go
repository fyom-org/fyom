package provider

import (
	"testing"
)

func TestParseS3Config(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string // empty means success
		check   func(t *testing.T, cfg S3Config)
	}{
		{
			name: "valid full config",
			raw: `{
				"bucket": "my-bucket",
				"region": "us-east-1",
				"access_key_id": "AKIA123",
				"secret_access_key": "secret",
				"endpoint": "https://s3.wasabisys.com",
				"path_prefix": "media/",
				"cdn_base_url": "https://cdn.example.com"
			}`,
			wantErr: "",
			check: func(t *testing.T, cfg S3Config) {
				if cfg.Bucket != "my-bucket" {
					t.Errorf("bucket = %q, want %q", cfg.Bucket, "my-bucket")
				}
				if cfg.Region != "us-east-1" {
					t.Errorf("region = %q, want %q", cfg.Region, "us-east-1")
				}
				if cfg.AccessKeyID != "AKIA123" {
					t.Errorf("access_key_id = %q, want %q", cfg.AccessKeyID, "AKIA123")
				}
				if cfg.SecretAccessKey != "secret" {
					t.Errorf("secret_access_key = %q, want %q", cfg.SecretAccessKey, "secret")
				}
				if cfg.Endpoint != "https://s3.wasabisys.com" {
					t.Errorf("endpoint = %q, want %q", cfg.Endpoint, "https://s3.wasabisys.com")
				}
				if cfg.PathPrefix != "media/" {
					t.Errorf("path_prefix = %q, want %q", cfg.PathPrefix, "media/")
				}
				if cfg.CDNBaseURL != "https://cdn.example.com" {
					t.Errorf("cdn_base_url = %q, want %q", cfg.CDNBaseURL, "https://cdn.example.com")
				}
			},
		},
		{
			name:    "valid minimal config",
			raw:     `{"bucket":"b","region":"us-west-2","access_key_id":"k","secret_access_key":"s"}`,
			wantErr: "",
			check: func(t *testing.T, cfg S3Config) {
				if cfg.Endpoint != "" {
					t.Errorf("endpoint should be empty, got %q", cfg.Endpoint)
				}
				if cfg.CDNBaseURL != "" {
					t.Errorf("cdn_base_url should be empty, got %q", cfg.CDNBaseURL)
				}
			},
		},
		{
			name:    "missing bucket",
			raw:     `{"region":"us-east-1","access_key_id":"k","secret_access_key":"s"}`,
			wantErr: "bucket is required",
		},
		{
			name:    "missing region",
			raw:     `{"bucket":"b","access_key_id":"k","secret_access_key":"s"}`,
			wantErr: "region is required",
		},
		{
			name:    "missing access_key_id",
			raw:     `{"bucket":"b","region":"us-east-1","secret_access_key":"s"}`,
			wantErr: "access_key_id is required",
		},
		{
			name:    "missing secret_access_key",
			raw:     `{"bucket":"b","region":"us-east-1","access_key_id":"k"}`,
			wantErr: "secret_access_key is required",
		},
		{
			name:    "invalid JSON",
			raw:     `{not json}`,
			wantErr: "invalid S3 config JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseS3Config(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestReplaceCDNHost(t *testing.T) {
	tests := []struct {
		name        string
		presignedURL string
		cdnBase     string
		want        string
		wantErr     string
	}{
		{
			name:        "AWS S3 to CDN",
			presignedURL: "https://bucket.s3.amazonaws.com/key?X-Amz-Signature=abc",
			cdnBase:     "https://cdn.example.com",
			want:        "https://cdn.example.com/key?X-Amz-Signature=abc",
		},
		{
			name:        "Wasabi to CDN",
			presignedURL: "https://s3.wasabisys.com/bucket/key?sig=x",
			cdnBase:     "https://mycdn.net",
			want:        "https://mycdn.net/bucket/key?sig=x",
		},
		{
			name:        "preserves complex query string",
			presignedURL: "https://bucket.s3.amazonaws.com/movies/Interstellar.mkv?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIA%2F20240101%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20240101T000000Z&X-Amz-Expires=3600&X-Amz-SignedHeaders=host&X-Amz-Signature=abcdef",
			cdnBase:     "https://cdn.example.com",
			want:        "https://cdn.example.com/movies/Interstellar.mkv?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIA%2F20240101%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20240101T000000Z&X-Amz-Expires=3600&X-Amz-SignedHeaders=host&X-Amz-Signature=abcdef",
		},
		{
			name:        "invalid presigned URL",
			presignedURL: "::bad",
			cdnBase:     "https://cdn.example.com",
			wantErr:     "invalid presigned URL",
		},
		{
			name:        "invalid CDN base",
			presignedURL: "https://bucket.s3.amazonaws.com/key",
			cdnBase:     "::bad",
			wantErr:     "invalid CDN base URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := replaceCDNHost(tt.presignedURL, tt.cdnBase)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("replaceCDNHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
