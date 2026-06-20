package provider

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fyom/fyom/internal/model"
)

// streamURLTTL is the lifetime of a presigned stream URL.
// Kept short because stream URLs may be shared or logged.
const streamURLTTL = 1 * time.Hour

// assetURLTTL is the lifetime of presigned poster/backdrop URLs.
// Longer TTL reduces re-signing overhead for static assets.
const assetURLTTL = 24 * time.Hour

// S3Provider generates presigned URLs for media stored in S3-compatible
// object storage. It implements the Provider interface.
// The Go server never proxies media bytes — the client streams directly
// from S3 using the presigned URL.
type S3Provider struct {
	id      string
	cfg     S3Config
	presign *s3.PresignClient
}

// NewS3Provider instantiates an S3Provider from a persisted ProviderRecord.
// It parses the config JSON, creates the AWS SDK client, and validates that
// all required fields are present. Returns an error if config is invalid.
// Does NOT make network calls — credentials are only verified at first use.
func NewS3Provider(rec model.ProviderRecord) (*S3Provider, error) {
	cfg, err := ParseS3Config(rec.Config)
	if err != nil {
		return nil, fmt.Errorf("S3Provider %q: %w", rec.ID, err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"", // session token — not used for static credentials
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("S3Provider %q: failed to load AWS config: %w", rec.ID, err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Path-style addressing is required for MinIO and most
			// S3-compatible services. Harmless for AWS S3.
			o.UsePathStyle = true
		}
	})

	return &S3Provider{
		id:      rec.ID,
		cfg:     cfg,
		presign: s3.NewPresignClient(s3Client),
	}, nil
}

// ID returns the provider identifier.
func (p *S3Provider) ID() string { return p.id }

// Type returns the provider type string.
func (p *S3Provider) Type() string { return "s3" }

// SupportsRedirect returns false: S3 presigned URLs are direct links that
// the client uses inline. No HTTP 302 is needed.
// (RemoteFyomProvider in Phase 5 will return true.)
func (p *S3Provider) SupportsRedirect() bool { return false }

// StreamURL generates a presigned URL for streaming the media item from S3.
func (p *S3Provider) StreamURL(ctx context.Context, item *model.MediaItem) (string, error) {
	if item.FilePath == "" {
		return "", nil
	}
	return p.presignKey(ctx, item.FilePath, streamURLTTL)
}

// PosterURL generates a presigned URL for the media item's poster image from S3.
func (p *S3Provider) PosterURL(ctx context.Context, item *model.MediaItem) (string, error) {
	if item.PosterPath == "" {
		return "", nil
	}
	return p.presignKey(ctx, item.PosterPath, assetURLTTL)
}

// BackdropURL generates a presigned URL for the media item's backdrop image from S3.
func (p *S3Provider) BackdropURL(ctx context.Context, item *model.MediaItem) (string, error) {
	if item.BackdropPath == "" {
		return "", nil
	}
	return p.presignKey(ctx, item.BackdropPath, assetURLTTL)
}

// LogoURL generates a presigned URL for the media item's logo image from S3.
func (p *S3Provider) LogoURL(ctx context.Context, item *model.MediaItem) (string, error) {
	if item.LogoPath == "" {
		return "", nil
	}
	return p.presignKey(ctx, item.LogoPath, assetURLTTL)
}

// presignKey generates a presigned GET URL for the given S3 object key.
// If CDNBaseURL is configured, the scheme+host of the URL is replaced
// with the CDN origin. The signature query string is preserved unchanged.
func (p *S3Provider) presignKey(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := p.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.cfg.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("S3Provider %q: presign key %q: %w", p.id, key, err)
	}

	rawURL := req.URL
	if p.cfg.CDNBaseURL == "" {
		return rawURL, nil
	}
	return replaceCDNHost(rawURL, p.cfg.CDNBaseURL)
}

// replaceCDNHost replaces the scheme+host of presignedURL with the
// scheme+host of cdnBase. The path and query string are unchanged.
func replaceCDNHost(presignedURL, cdnBase string) (string, error) {
	parsed, err := url.Parse(presignedURL)
	if err != nil {
		return "", fmt.Errorf("replaceCDNHost: invalid presigned URL: %w", err)
	}
	cdn, err := url.Parse(cdnBase)
	if err != nil {
		return "", fmt.Errorf("replaceCDNHost: invalid CDN base URL: %w", err)
	}
	parsed.Scheme = cdn.Scheme
	parsed.Host = cdn.Host
	return parsed.String(), nil
}
