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
	// NOTE: If UseWorkerPool is true, BaseURL is ignored (workers use Unix sockets).
	BaseURL string
	// UseWorkerPool enables the fast worker pool with Unix sockets (recommended).
	// If false, falls back to HTTP client.
	UseWorkerPool bool
	// MaxWorkers is the number of Python worker subprocesses to maintain (default: 4).
	// Only used if UseWorkerPool is true.
	MaxWorkers int
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

	// Use worker pool by default (fast, no HTTP)
	useWorkerPool := cfg.UseWorkerPool
	if !cfg.UseWorkerPool && cfg.BaseURL == "" {
		// Default to worker pool if no BaseURL specified
		useWorkerPool = true
	}

	if useWorkerPool {
		// Use fast worker pool with Unix sockets
		maxWorkers := cfg.MaxWorkers
		if maxWorkers == 0 {
			maxWorkers = 4 // Default: 4 workers
		}

		cfg.Logger.WithFields(logrus.Fields{
			"engine":     cfg.Engine,
			"max_workers": maxWorkers,
			"method":     "worker_pool_unix_socket",
		}).Info("Creating translator with worker pool")

		return NewWorkerPool(cfg.Engine, maxWorkers, cfg.Logger)
	}

	// Fall back to HTTP client (legacy mode)
	// Set default base URL if not provided
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:5000"
	}

	cfg.Logger.WithFields(logrus.Fields{
		"engine":   cfg.Engine,
		"base_url": cfg.BaseURL,
		"method":   "http_client",
	}).Info("Creating translator with HTTP client")

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

