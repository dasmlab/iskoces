package translate

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// EngineType represents the type of translation engine to use.
type EngineType string

const (
	// EngineLibreTranslate uses LibreTranslate as the backend.
	EngineLibreTranslate EngineType = "libretranslate"
	// EngineArgos uses Argos Translate as the backend.
	EngineArgos EngineType = "argos"
)

// Config holds configuration for creating a Translator instance.
type Config struct {
	// Engine specifies which translation engine to use.
	Engine EngineType
	// BaseURL is the base URL for the translation engine API.
	// Defaults to http://127.0.0.1:5000 if not specified.
	BaseURL string
	// Logger is the logger instance to use. If nil, a default logger is created.
	Logger *logrus.Logger
}

// NewTranslator creates a new Translator instance based on the configuration.
// This factory function allows switching between different MT backends
// without changing the gRPC service implementation.
func NewTranslator(cfg Config) (Translator, error) {
	if cfg.Logger == nil {
		cfg.Logger = logrus.New()
	}

	// Set default base URL if not provided
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:5000"
	}

	cfg.Logger.WithFields(logrus.Fields{
		"engine":  cfg.Engine,
		"base_url": cfg.BaseURL,
	}).Info("Creating translator instance")

	switch cfg.Engine {
	case EngineLibreTranslate:
		return NewLibreTranslateClient(cfg.BaseURL, cfg.Logger), nil
	case EngineArgos:
		return NewArgosClient(cfg.BaseURL, cfg.Logger), nil
	default:
		cfg.Logger.WithFields(logrus.Fields{
			"engine": cfg.Engine,
		}).Error("Unknown translation engine")
		return nil, fmt.Errorf("unknown translation engine: %s", cfg.Engine)
	}
}

// ParseEngineType parses a string into an EngineType.
// Returns an error if the string is not a valid engine type.
func ParseEngineType(s string) (EngineType, error) {
	switch s {
	case "libretranslate", "LibreTranslate", "LIBRETRANSLATE":
		return EngineLibreTranslate, nil
	case "argos", "Argos", "ARGOS":
		return EngineArgos, nil
	default:
		return "", fmt.Errorf("unknown engine type: %s (supported: libretranslate, argos)", s)
	}
}

