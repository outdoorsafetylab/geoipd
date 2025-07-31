package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCSStorage implements CloudStorage for Google Cloud Storage
type GCSStorage struct {
	client    *storage.Client
	bucket    string
	keyPrefix string
}

// NewGCSStorage creates a new GCS storage instance
func NewGCSStorage(config *Config) (*GCSStorage, error) {
	if config.Bucket == "" {
		return nil, ErrInvalidConfig
	}

	ctx := context.Background()

	// Use Application Default Credentials (ADC) for authentication
	// This works automatically in Cloud Run and when gcloud is configured locally
	client, err := storage.NewClient(ctx, option.WithScopes(storage.ScopeReadWrite))
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSStorage{
		client:    client,
		bucket:    config.Bucket,
		keyPrefix: config.KeyPrefix,
	}, nil
}

// UploadWithMetadata uploads data with metadata to GCS
func (g *GCSStorage) UploadWithMetadata(key string, data io.Reader, metadata map[string]string) error {
	ctx := context.Background()
	fullKey := g.getFullKey(key)

	obj := g.client.Bucket(g.bucket).Object(fullKey)
	writer := obj.NewWriter(ctx)

	// Set metadata
	if metadata != nil {
		writer.Metadata = metadata
	}

	// Copy data
	if _, err := io.Copy(writer, data); err != nil {
		writer.Close()
		return fmt.Errorf("failed to upload to GCS: %w", err)
	}

	return writer.Close()
}

// Download downloads data from GCS
func (g *GCSStorage) Download(key string) (io.ReadCloser, error) {
	ctx := context.Background()
	fullKey := g.getFullKey(key)

	obj := g.client.Bucket(g.bucket).Object(fullKey)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("failed to download from GCS: %w", err)
	}

	return reader, nil
}

// GetMetadata retrieves metadata for a GCS object
func (g *GCSStorage) GetMetadata(key string) (map[string]string, error) {
	ctx := context.Background()
	fullKey := g.getFullKey(key)

	obj := g.client.Bucket(g.bucket).Object(fullKey)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("failed to get GCS metadata: %w", err)
	}

	return attrs.Metadata, nil
}

// Exists checks if an object exists in GCS
func (g *GCSStorage) Exists(key string) (bool, error) {
	ctx := context.Background()
	fullKey := g.getFullKey(key)

	obj := g.client.Bucket(g.bucket).Object(fullKey)
	_, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, fmt.Errorf("failed to check GCS object existence: %w", err)
	}

	return true, nil
}

// GetLastModified returns the last modified time of a GCS object
func (g *GCSStorage) GetLastModified(key string) (time.Time, error) {
	ctx := context.Background()
	fullKey := g.getFullKey(key)

	obj := g.client.Bucket(g.bucket).Object(fullKey)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return time.Time{}, ErrObjectNotFound
		}
		return time.Time{}, fmt.Errorf("failed to get GCS object attributes: %w", err)
	}

	return attrs.Updated, nil
}

// getFullKey combines the key prefix with the object key
func (g *GCSStorage) getFullKey(key string) string {
	if g.keyPrefix == "" {
		return key
	}
	return strings.TrimSuffix(g.keyPrefix, "/") + "/" + key
}
