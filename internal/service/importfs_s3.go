package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/provider"
)

// S3ImportFS implements ImportFS for S3-compatible object storage.
type S3ImportFS struct {
	client *s3.Client
	bucket string
	prefix string // root prefix for this import (e.g. "Shows/")
}

// NewS3ImportFS creates an S3-backed ImportFS.
// prefix is the S3 key prefix that acts as the "root directory" for the import.
// It must end with "/" — added automatically if missing.
func NewS3ImportFS(ctx context.Context, rec *model.ProviderRecord, prefix string) (*S3ImportFS, error) {
	cfgData, err := provider.ParseS3Config(rec.Config)
	if err != nil {
		return nil, fmt.Errorf("parse S3 config: %w", err)
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfgData.AccessKeyID, cfgData.SecretAccessKey, "",
		)),
		config.WithRegion(cfgData.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfgData.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfgData.Endpoint)
			o.UsePathStyle = true
		}
	})

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &S3ImportFS{
		client: client,
		bucket: cfgData.Bucket,
		prefix: prefix,
	}, nil
}

// s3FullKey converts a logical path (relative to the import root) to a full S3 key.
func (fs *S3ImportFS) s3FullKey(logicalPath string) string {
	logicalPath = strings.TrimPrefix(logicalPath, "/")
	return fs.prefix + logicalPath
}

// s3LogicalPath converts a full S3 key back to a logical path
// (relative to the import root prefix).
func (fs *S3ImportFS) s3LogicalPath(fullKey string) string {
	return strings.TrimPrefix(fullKey, fs.prefix)
}

// ReadDir returns a sorted list of directory entries for the given S3 prefix.
func (fs *S3ImportFS) ReadDir(ctx context.Context, dir string) ([]DirEntry, error) {
	prefix := fs.s3FullKey(dir)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var entries []DirEntry
	paginator := s3.NewListObjectsV2Paginator(fs.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(fs.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list S3 objects: %w", err)
		}

		// Common prefixes = "directories"
		for _, cp := range page.CommonPrefixes {
			name := strings.TrimSuffix(fs.s3LogicalPath(*cp.Prefix), "/")
			name = filepath.Base(name)
			entries = append(entries, DirEntry{Name: name, IsDir: true})
		}

		// Objects = "files" directly under this prefix
		for _, obj := range page.Contents {
			key := *obj.Key
			// Skip the directory marker itself
			if key == prefix {
				continue
			}
			logical := fs.s3LogicalPath(key)
			// Only include files directly in this dir (no further slashes)
			relPart := strings.TrimPrefix(logical, strings.TrimSuffix(dir, "/")+"/")
			if strings.Contains(relPart, "/") {
				continue
			}
			name := filepath.Base(logical)
			entries = append(entries, DirEntry{Name: name, IsDir: false})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// Open downloads the named object from S3 and returns its body as a ReadCloser.
func (fs *S3ImportFS) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	key := fs.s3FullKey(name)
	resp, err := fs.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get S3 object %s: %w", key, err)
	}
	return resp.Body, nil
}

// Exists reports whether the named object exists in the S3 bucket.
func (fs *S3ImportFS) Exists(ctx context.Context, name string) bool {
	key := fs.s3FullKey(name)
	_, err := fs.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(key),
	})
	return err == nil
}

// Join joins any number of path elements into a single forward-slash-separated path.
func (fs *S3ImportFS) Join(elem ...string) string {
	joined := strings.Join(elem, "/")
	// Replace double slashes (but not :// in URLs)
	for strings.Contains(joined, "//") {
		joined = strings.ReplaceAll(joined, "//", "/")
	}
	return joined
}
