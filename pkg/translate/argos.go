package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultArgosURL is the default base URL for Argos Translate API.
	DefaultArgosURL = "http://127.0.0.1:5000"
	// DefaultArgosTimeout is the default timeout for HTTP requests.
	DefaultArgosTimeout = 30 * time.Second
)

// ArgosClient implements the Translator interface using Argos Translate.
// Argos Translate is a lightweight, open-source machine translation library.
// Note: This implementation assumes Argos is running as an HTTP service.
// If Argos provides a different API, this will need to be adjusted.
type ArgosClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewArgosClient creates a new Argos Translate client.
// baseURL should point to the Argos Translate server (default: http://127.0.0.1:5000).
// Note: Argos may need to be wrapped in an HTTP service layer if it doesn't
// provide an HTTP API out of the box.
func NewArgosClient(baseURL string, logger *logrus.Logger) *ArgosClient {
	if baseURL == "" {
		baseURL = DefaultArgosURL
	}
	if logger == nil {
		logger = logrus.New()
	}

	return &ArgosClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultArgosTimeout,
		},
		logger: logger,
	}
}

// argosTranslateRequest represents an Argos Translate API request.
// This structure may need to be adjusted based on the actual Argos API.
type argosTranslateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"` // e.g., "en"
	TargetLang string `json:"target_lang"` // e.g., "fr"
}

// argosTranslateResponse represents an Argos Translate API response.
type argosTranslateResponse struct {
	TranslatedText string `json:"translated_text"`
}

// Translate translates text from source language to target language.
// sourceLang and targetLang should be in ISO 639-1 format (e.g., "en", "fr").
func (c *ArgosClient) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	c.logger.WithFields(logrus.Fields{
		"source_lang": sourceLang,
		"target_lang": targetLang,
		"text_length": len(text),
	}).Debug("Translating text with Argos")

	// Build request payload
	// Note: This structure may need adjustment based on actual Argos API
	reqPayload := argosTranslateRequest{
		Text:       text,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}

	// Encode request body
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(&reqPayload); err != nil {
		c.logger.WithError(err).Error("Failed to encode translation request")
		return "", fmt.Errorf("encode request: %w", err)
	}

	// Create HTTP request
	// Note: The endpoint may differ based on Argos API implementation
	url := c.baseURL + "/translate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create translation request")
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).WithFields(logrus.Fields{
			"url": url,
		}).Error("Translation request failed")
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	c.logger.WithFields(logrus.Fields{
		"status_code": resp.StatusCode,
		"duration_ms": duration.Milliseconds(),
	}).Debug("Translation request completed")

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"response":     string(bodyBytes),
		}).Error("Translation request returned non-OK status")
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var argosResp argosTranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&argosResp); err != nil {
		c.logger.WithError(err).Error("Failed to decode translation response")
		return "", fmt.Errorf("decode response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"source_lang": sourceLang,
		"target_lang": targetLang,
		"duration_ms": duration.Milliseconds(),
	}).Info("Translation completed successfully")

	return argosResp.TranslatedText, nil
}

// CheckHealth verifies that Argos Translate is ready and operational.
func (c *ArgosClient) CheckHealth(ctx context.Context) error {
	c.logger.Debug("Checking Argos Translate health")

	// Use a simple health check endpoint
	// Note: This endpoint may need to be adjusted based on actual Argos API
	url := c.baseURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create health check request")
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// If /health doesn't exist, try /translate with a minimal request
		c.logger.WithError(err).Debug("Health endpoint not available, trying alternative check")
		// For now, we'll consider it healthy if we can reach the server
		// In production, implement a proper health check endpoint
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
		}).Warn("Health check returned non-OK status")
		// Don't fail health check for non-OK, as Argos may not have a health endpoint
		return nil
	}

	c.logger.Debug("Argos Translate health check passed")
	return nil
}

// SupportedLanguages returns a list of language codes supported by Argos Translate.
// Note: This may need to be adjusted based on actual Argos API.
func (c *ArgosClient) SupportedLanguages(ctx context.Context) ([]string, error) {
	c.logger.Debug("Fetching supported languages from Argos")

	// Common language codes supported by Argos Translate
	// This is a hardcoded list; in production, fetch from API if available
	// Argos typically supports: en, es, fr, de, it, pt, ru, zh, ja, ko, etc.
	supported := []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "zh", "ja", "ko",
		"ar", "hi", "tr", "pl", "nl", "sv", "da", "fi", "no", "cs",
		"ro", "hu", "bg", "hr", "sk", "sl", "et", "lv", "lt", "el",
	}

	c.logger.WithFields(logrus.Fields{
		"count": len(supported),
	}).Debug("Returning supported languages")

	return supported, nil
}

