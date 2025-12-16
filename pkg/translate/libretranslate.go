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
	// DefaultLibreTranslateURL is the default base URL for LibreTranslate API.
	DefaultLibreTranslateURL = "http://localhost:5000"
	// DefaultLibreTranslateTimeout is the default timeout for HTTP requests.
	// Increased to 5 minutes to handle large documents that may take longer to translate.
	DefaultLibreTranslateTimeout = 5 * time.Minute
)

// LibreTranslateClient implements the Translator interface using LibreTranslate.
// LibreTranslate is a self-hosted, open-source machine translation API.
type LibreTranslateClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewLibreTranslateClient creates a new LibreTranslate client.
// baseURL should point to the LibreTranslate server (default: http://127.0.0.1:5000).
func NewLibreTranslateClient(baseURL string, logger *logrus.Logger) *LibreTranslateClient {
	if baseURL == "" {
		baseURL = DefaultLibreTranslateURL
	}
	if logger == nil {
		logger = logrus.New()
	}

	return &LibreTranslateClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultLibreTranslateTimeout,
		},
		logger: logger,
	}
}

// translateRequest represents a LibreTranslate API request.
type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"` // e.g., "en"
	Target string `json:"target"` // e.g., "fr"
	Format string `json:"format"` // "text" or "html"
}

// translateResponse represents a LibreTranslate API response.
type translateResponse struct {
	TranslatedText string `json:"translatedText"`
}

// languagesResponse represents the response from the /languages endpoint.
type languagesResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Translate translates text from source language to target language.
// sourceLang and targetLang should be in ISO 639-1 format (e.g., "en", "fr").
func (c *LibreTranslateClient) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	c.logger.WithFields(logrus.Fields{
		"source_lang": sourceLang,
		"target_lang": targetLang,
		"text_length": len(text),
	}).Debug("Translating text with LibreTranslate")

	// Build request payload
	reqPayload := translateRequest{
		Q:      text,
		Source: sourceLang,
		Target: targetLang,
		Format: "text",
	}

	// Encode request body
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(&reqPayload); err != nil {
		c.logger.WithError(err).Error("Failed to encode translation request")
		return "", fmt.Errorf("encode request: %w", err)
	}

	// Create HTTP request
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
			"response":    string(bodyBytes),
		}).Error("Translation request returned non-OK status")
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Decode response
	var ltResp translateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ltResp); err != nil {
		c.logger.WithError(err).Error("Failed to decode translation response")
		return "", fmt.Errorf("decode response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"source_lang": sourceLang,
		"target_lang": targetLang,
		"duration_ms": duration.Milliseconds(),
	}).Info("Translation completed successfully")

	return ltResp.TranslatedText, nil
}

// CheckHealth verifies that LibreTranslate is ready and operational.
func (c *LibreTranslateClient) CheckHealth(ctx context.Context) error {
	c.logger.Debug("Checking LibreTranslate health")

	// Use the /languages endpoint as a health check
	url := c.baseURL + "/languages"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create health check request")
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).WithFields(logrus.Fields{
			"url": url,
		}).Error("Health check request failed")
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
		}).Error("Health check returned non-OK status")
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	c.logger.Debug("LibreTranslate health check passed")
	return nil
}

// SupportedLanguages returns a list of language codes supported by LibreTranslate.
func (c *LibreTranslateClient) SupportedLanguages(ctx context.Context) ([]string, error) {
	c.logger.Debug("Fetching supported languages from LibreTranslate")

	url := c.baseURL + "/languages"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create languages request")
		return nil, fmt.Errorf("create languages request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("Failed to fetch supported languages")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
		}).Error("Languages request returned non-OK status")
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var languages []languagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&languages); err != nil {
		c.logger.WithError(err).Error("Failed to decode languages response")
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Extract language codes
	codes := make([]string, 0, len(languages))
	for _, lang := range languages {
		codes = append(codes, lang.Code)
	}

	c.logger.WithFields(logrus.Fields{
		"count": len(codes),
	}).Debug("Fetched supported languages")

	return codes, nil
}
