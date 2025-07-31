package storage

import "errors"

var (
	// ErrUnsupportedProvider is returned when an unsupported storage provider is specified
	ErrUnsupportedProvider = errors.New("unsupported storage provider")

	// ErrObjectNotFound is returned when an object doesn't exist in storage
	ErrObjectNotFound = errors.New("object not found")

	// ErrInvalidConfig is returned when storage configuration is invalid
	ErrInvalidConfig = errors.New("invalid storage configuration")
)
