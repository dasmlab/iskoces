package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	nanabushv1 "github.com/dasmlab/iskoces/pkg/proto/v1"
	"github.com/dasmlab/iskoces/pkg/translate"
	"github.com/sirupsen/logrus"
)

// JobProcessor processes translation jobs asynchronously.
type JobProcessor struct {
	translator     translate.Translator
	languageMapper *translate.LanguageMapper
	logger         *logrus.Logger
	chunkSize      int // Maximum chunk size in bytes (default: 10KB)
}

// NewJobProcessor creates a new job processor.
func NewJobProcessor(translator translate.Translator, languageMapper *translate.LanguageMapper, logger *logrus.Logger) *JobProcessor {
	return &JobProcessor{
		translator:     translator,
		languageMapper: languageMapper,
		logger:         logger,
		chunkSize:      10 * 1024, // 10KB default
	}
}

// ProcessJob processes a translation job asynchronously.
func (p *JobProcessor) ProcessJob(job *TranslationJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	startTime := time.Now()
	
	p.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"request_id": job.RequestID,
		"primitive":  job.Primitive.String(),
	}).Info("Starting translation job processing")

	job.UpdateStatus(JobStatusProcessing, "Starting translation...")

	// Convert language codes
	sourceLang := p.languageMapper.ToBackendCode(job.SourceLang)
	targetLang := p.languageMapper.ToBackendCode(job.TargetLang)

	var translatedTitle string
	var translatedMarkdown string
	var err error

	// Handle different primitive types
	switch job.Primitive {
	case nanabushv1.PrimitiveType_PRIMITIVE_TITLE:
		// Title-only translation
		job.UpdateProgress(10, "Translating title...")
		if p.translator != nil {
			translatedTitle, err = p.translator.Translate(ctx, job.Title, sourceLang, targetLang)
			if err != nil {
				p.logger.WithError(err).WithFields(logrus.Fields{
					"job_id": job.ID,
				}).Error("Title translation failed")
				job.SetError(fmt.Errorf("title translation failed: %w", err))
				return
			}
		}
		job.UpdateProgress(100, "Translation completed")

	case nanabushv1.PrimitiveType_PRIMITIVE_DOC_TRANSLATE:
		// Full document translation
		if job.Document == nil {
			job.SetError(fmt.Errorf("document is required for PRIMITIVE_DOC_TRANSLATE"))
			return
		}

		// Translate title if present
		if job.Document.Title != "" {
			job.UpdateProgress(5, "Translating title...")
			if p.translator != nil {
				translatedTitle, err = p.translator.Translate(ctx, job.Document.Title, sourceLang, targetLang)
				if err != nil {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"job_id": job.ID,
					}).Error("Title translation failed")
					job.SetError(fmt.Errorf("title translation failed: %w", err))
					return
				}
			}
		}

		// Translate markdown content
		markdown := job.Document.Markdown
		if markdown != "" {
			job.UpdateProgress(10, "Translating content...")
			
			// Check if we need to chunk the content
			if len(markdown) > p.chunkSize {
				translatedMarkdown, err = p.translateChunked(ctx, markdown, sourceLang, targetLang, job)
				if err != nil {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"job_id": job.ID,
					}).Error("Chunked translation failed")
					job.SetError(fmt.Errorf("markdown translation failed: %w", err))
					return
				}
			} else {
				// Small enough to translate in one go
				if p.translator != nil {
					translatedMarkdown, err = p.translator.Translate(ctx, markdown, sourceLang, targetLang)
					if err != nil {
						p.logger.WithError(err).WithFields(logrus.Fields{
							"job_id": job.ID,
						}).Error("Markdown translation failed")
						job.SetError(fmt.Errorf("markdown translation failed: %w", err))
						return
					}
				}
			}
		}

		job.UpdateProgress(100, "Translation completed")
	}

	// Calculate inference time
	inferenceTime := time.Since(startTime).Seconds()

	// Set result
	job.SetResult(translatedTitle, translatedMarkdown, 0, inferenceTime)

	p.logger.WithFields(logrus.Fields{
		"job_id":         job.ID,
		"request_id":     job.RequestID,
		"inference_time": inferenceTime,
		"success":        true,
	}).Info("Translation job completed successfully")
}

// translateChunked translates large content by splitting it into chunks.
// This helps avoid timeouts and allows progress updates.
func (p *JobProcessor) translateChunked(ctx context.Context, text string, sourceLang, targetLang string, job *TranslationJob) (string, error) {
	p.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"text_length": len(text),
		"chunk_size":  p.chunkSize,
	}).Info("Translating large document in chunks")

	// Split text into chunks at sentence boundaries (prefer) or word boundaries
	chunks := p.splitIntoChunks(text, p.chunkSize)
	totalChunks := len(chunks)
	
	p.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"total_chunks": totalChunks,
	}).Info("Split document into chunks")

	var translatedChunks []string
	
	for i, chunk := range chunks {
		// Update progress (10% to 90% for content translation)
		progress := 10 + int32((float64(i+1)/float64(totalChunks))*80)
		job.UpdateProgress(progress, fmt.Sprintf("Translating chunk %d/%d...", i+1, totalChunks))
		
		if p.translator != nil {
			translated, err := p.translator.Translate(ctx, chunk, sourceLang, targetLang)
			if err != nil {
				return "", fmt.Errorf("chunk %d translation failed: %w", i+1, err)
			}
			translatedChunks = append(translatedChunks, translated)
		}
	}

	// Join translated chunks
	result := strings.Join(translatedChunks, "")
	
	p.logger.WithFields(logrus.Fields{
		"job_id":           job.ID,
		"original_length": len(text),
		"translated_length": len(result),
		"chunks":           totalChunks,
	}).Info("Chunked translation completed")

	return result, nil
}

// splitIntoChunks splits text into chunks, trying to break at sentence boundaries.
func (p *JobProcessor) splitIntoChunks(text string, maxChunkSize int) []string {
	if len(text) <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	currentChunk := ""
	
	// Split by paragraphs first (double newline)
	paragraphs := strings.Split(text, "\n\n")
	
	for _, para := range paragraphs {
		// If adding this paragraph would exceed chunk size, save current chunk and start new one
		if len(currentChunk)+len(para)+2 > maxChunkSize && currentChunk != "" {
			chunks = append(chunks, currentChunk)
			currentChunk = ""
		}
		
		// If paragraph itself is too large, split by sentences
		if len(para) > maxChunkSize {
			// Split current chunk if it has content
			if currentChunk != "" {
				chunks = append(chunks, currentChunk)
				currentChunk = ""
			}
			
			// Split paragraph by sentences
			sentences := p.splitBySentences(para)
			for _, sentence := range sentences {
				if len(currentChunk)+len(sentence)+1 > maxChunkSize && currentChunk != "" {
					chunks = append(chunks, currentChunk)
					currentChunk = ""
				}
				if currentChunk != "" {
					currentChunk += " "
				}
				currentChunk += sentence
			}
		} else {
			// Paragraph fits, add it
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
	}
	
	// Add remaining chunk
	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}
	
	return chunks
}

// splitBySentences splits text by sentence boundaries (., !, ? followed by space or newline).
func (p *JobProcessor) splitBySentences(text string) []string {
	var sentences []string
	current := ""
	
	for i, r := range text {
		current += string(r)
		
		// Check for sentence ending
		if (r == '.' || r == '!' || r == '?') && i+1 < len(text) {
			next := text[i+1]
			if next == ' ' || next == '\n' || next == '\t' {
				sentences = append(sentences, strings.TrimSpace(current))
				current = ""
			}
		}
	}
	
	// Add remaining text
	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}
	
	return sentences
}

