package storage

import (
	"errors"
	"io"
	"time"
)

// CloudStorage defines the interface for cloud storage operations
type CloudStorage interface {
	// UploadWithMetadata uploads data with metadata
	UploadWithMetadata(key string, data io.Reader, metadata map[string]string) error

	// Download downloads data from storage
	Download(key string) (io.ReadCloser, error)

	// GetMetadata retrieves metadata for an object
	GetMetadata(key string) (map[string]string, error)

	// Exists checks if an object exists
	Exists(key string) (bool, error)

	// GetLastModified returns the last modified time of an object
	GetLastModified(key string) (time.Time, error)
}

// Config represents cloud storage configuration
type Config struct {
	Provider  string `mapstructure:"provider"` // currently only "gcs" is supported
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"` // for future s3/azure support
	KeyPrefix string `mapstructure:"key_prefix"`
}

// NewCloudStorage creates a new cloud storage instance based on provider
func NewCloudStorage(config *Config) (CloudStorage, error) {
	switch config.Provider {
	case "gcs":
		return NewGCSStorage(config)
	case "s3", "azure":
		return nil, errors.New("only GCS is currently supported - S3 and Azure are not implemented")
	default:
		return nil, ErrUnsupportedProvider
	}
}
