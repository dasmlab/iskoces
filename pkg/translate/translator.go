package translate

import (
	"context"
	"strings"
)

// Translator defines the interface for machine translation backends.
// This abstraction allows us to switch between different MT engines
// (LibreTranslate, Argos, etc.) without changing the gRPC service implementation.
type Translator interface {
	// Translate translates text from source language to target language.
	// sourceLang and targetLang should be in ISO 639-1 format (e.g., "en", "fr").
	Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error)

	// CheckHealth verifies that the translation backend is ready and operational.
	CheckHealth(ctx context.Context) error

	// SupportedLanguages returns a list of language codes supported by this backend.
	// Returns ISO 639-1 codes (e.g., ["en", "fr", "es"]).
	SupportedLanguages(ctx context.Context) ([]string, error)
}

// LanguageMapper handles conversion between different language code formats.
// Proto uses formats like "EN" and "fr-CA" (BCP 47), while backends typically
// use ISO 639-1 codes like "en" and "fr".
type LanguageMapper struct{}

// NewLanguageMapper creates a new language mapper instance.
func NewLanguageMapper() *LanguageMapper {
	return &LanguageMapper{}
}

// ToBackendCode converts a proto language code to backend format.
// Examples:
//   - "EN" -> "en"
//   - "fr-CA" -> "fr"
//   - "en-US" -> "en"
func (lm *LanguageMapper) ToBackendCode(protoLang string) string {
	// Convert to lowercase and extract base language code
	// Handle BCP 47 tags by taking the first part before "-"
	lang := strings.ToLower(protoLang)
	
	// Extract base language (before any "-" or "_")
	if idx := strings.IndexAny(lang, "-_"); idx >= 0 {
		lang = lang[:idx]
	}
	
	return lang
}

