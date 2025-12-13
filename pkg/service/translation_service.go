package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dasmlab/iskoces/pkg/proto/v1"
	"github.com/dasmlab/iskoces/pkg/translate"
	"github.com/sirupsen/logrus"
)

// ClientInfo tracks registered client information.
type ClientInfo struct {
	ClientID      string
	ClientName    string
	ClientVersion string
	Namespace     string
	Metadata      map[string]string
	RegisteredAt  time.Time
	LastHeartbeat time.Time
}

// TranslationService implements the TranslationService gRPC service.
// This service provides translation capabilities using lightweight MT backends
// (LibreTranslate or Argos) instead of the heavier vLLM used by Nanabush.
type TranslationService struct {
	nanabushv1.UnimplementedTranslationServiceServer

	// Translator is the translation backend (LibreTranslate or Argos).
	Translator translate.Translator

	// LanguageMapper handles conversion between proto language codes and backend codes.
	LanguageMapper *translate.LanguageMapper

	// Logger for service operations.
	Logger *logrus.Logger

	// Client tracking for registration and heartbeat management.
	clients         map[string]*ClientInfo
	clientsMutex    sync.RWMutex
	clientIDCounter int64
	heartbeatInterval int32 // seconds
}

// NewTranslationService creates a new TranslationService instance.
func NewTranslationService(translator translate.Translator, logger *logrus.Logger) *TranslationService {
	if logger == nil {
		logger = logrus.New()
	}

	return &TranslationService{
		Translator:     translator,
		LanguageMapper: translate.NewLanguageMapper(),
		Logger:         logger,
		clients:        make(map[string]*ClientInfo),
		heartbeatInterval: 30, // Default: 30 seconds
	}
}

// RegisterClient registers a new client with the server.
// This should be called immediately after establishing a gRPC connection.
func (s *TranslationService) RegisterClient(ctx context.Context, req *nanabushv1.RegisterClientRequest) (*nanabushv1.RegisterClientResponse, error) {
	s.Logger.WithFields(logrus.Fields{
		"client_name":    req.ClientName,
		"client_version": req.ClientVersion,
		"namespace":     req.Namespace,
		"metadata":      req.Metadata,
	}).Info("[gRPC] RegisterClient request received")

	// Validate request
	if req.ClientName == "" {
		s.Logger.Error("[gRPC] RegisterClient: client_name is required")
		return nil, status.Error(codes.InvalidArgument, "client_name is required")
	}

	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	// Generate unique client ID
	s.clientIDCounter++
	clientID := fmt.Sprintf("iskoces-client-%d-%d", time.Now().Unix(), s.clientIDCounter)

	now := time.Now()
	// Create client info
	clientInfo := &ClientInfo{
		ClientID:      clientID,
		ClientName:    req.ClientName,
		ClientVersion: req.ClientVersion,
		Namespace:     req.Namespace,
		Metadata:      req.Metadata,
		RegisteredAt:  now,
		LastHeartbeat: now,
	}

	// Store client
	s.clients[clientID] = clientInfo

	s.Logger.WithFields(logrus.Fields{
		"client_id":     clientID,
		"client_name":   req.ClientName,
		"total_clients": len(s.clients),
	}).Info("[gRPC] Client registered successfully, sending response")

	// Calculate expiration (24 hours from now)
	expiresAt := now.Add(24 * time.Hour)

	response := &nanabushv1.RegisterClientResponse{
		ClientId:               clientID,
		Success:                true,
		Message:                fmt.Sprintf("Client %q registered successfully", req.ClientName),
		HeartbeatIntervalSeconds: int32(s.heartbeatInterval),
		ExpiresAt:              timestamppb.New(expiresAt),
	}

	s.Logger.WithFields(logrus.Fields{
		"client_id":                clientID,
		"heartbeat_interval_sec":   s.heartbeatInterval,
		"expires_at":               expiresAt.Format(time.RFC3339),
	}).Info("[gRPC] RegisterClient response prepared, returning to client")

	return response, nil
}

// Heartbeat sends a keepalive and re-authentication signal from the client.
// Should be called periodically to maintain the connection.
func (s *TranslationService) Heartbeat(ctx context.Context, req *nanabushv1.HeartbeatRequest) (*nanabushv1.HeartbeatResponse, error) {
	s.Logger.WithFields(logrus.Fields{
		"client_id":   req.ClientId,
		"client_name": req.ClientName,
	}).Debug("[gRPC] Heartbeat request received")

	// Validate request
	if req.ClientId == "" {
		s.Logger.Error("Heartbeat: client_id is required")
		return nil, status.Error(codes.InvalidArgument, "client_id is required")
	}
	if req.ClientName == "" {
		s.Logger.Error("Heartbeat: client_name is required")
		return nil, status.Error(codes.InvalidArgument, "client_name is required")
	}

	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	// Look up client
	clientInfo, exists := s.clients[req.ClientId]
	if !exists {
		s.Logger.WithFields(logrus.Fields{
			"client_id":   req.ClientId,
			"client_name": req.ClientName,
		}).Warn("Heartbeat from unknown client")
		return &nanabushv1.HeartbeatResponse{
			Success:             false,
			Message:             "Client not registered or expired",
			ReceivedAt:          timestamppb.Now(),
			HeartbeatIntervalSeconds: int32(s.heartbeatInterval),
			ReRegisterRequired: true,
		}, nil
	}

	// Validate client name matches
	if clientInfo.ClientName != req.ClientName {
		s.Logger.WithFields(logrus.Fields{
			"expected": clientInfo.ClientName,
			"got":      req.ClientName,
		}).Warn("Heartbeat client name mismatch")
		return &nanabushv1.HeartbeatResponse{
			Success:             false,
			Message:             "Client name mismatch",
			ReceivedAt:          timestamppb.Now(),
			HeartbeatIntervalSeconds: int32(s.heartbeatInterval),
			ReRegisterRequired: true,
		}, nil
	}

	// Update last heartbeat time
	clientInfo.LastHeartbeat = time.Now()

	// Check if registration expired (24 hours)
	if time.Since(clientInfo.RegisteredAt) > 24*time.Hour {
		s.Logger.WithFields(logrus.Fields{
			"client_id":   req.ClientId,
			"client_name": req.ClientName,
		}).Warn("Client registration expired")
		delete(s.clients, req.ClientId)
		return &nanabushv1.HeartbeatResponse{
			Success:             false,
			Message:             "Registration expired",
			ReceivedAt:          timestamppb.Now(),
			HeartbeatIntervalSeconds: int32(s.heartbeatInterval),
			ReRegisterRequired: true,
		}, nil
	}

	s.Logger.WithFields(logrus.Fields{
		"client_id":     req.ClientId,
		"client_name":   req.ClientName,
		"last_seen":     clientInfo.LastHeartbeat,
	}).Debug("Heartbeat acknowledged")

	return &nanabushv1.HeartbeatResponse{
		Success:             true,
		Message:             "Heartbeat acknowledged",
		ReceivedAt:          timestamppb.Now(),
		HeartbeatIntervalSeconds: int32(s.heartbeatInterval),
		ReRegisterRequired: false,
	}, nil
}

// CheckTitle performs a lightweight pre-flight check with title only.
// This validates that Iskoces is ready and can handle the request.
func (s *TranslationService) CheckTitle(ctx context.Context, req *nanabushv1.TitleCheckRequest) (*nanabushv1.TitleCheckResponse, error) {
	s.Logger.WithFields(logrus.Fields{
		"title":          req.Title,
		"source_lang":    req.SourceLanguage,
		"target_lang":   req.LanguageTag,
	}).Debug("CheckTitle request received")

	// Validate request
	if req.Title == "" {
		s.Logger.Error("CheckTitle: title is required")
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.LanguageTag == "" {
		s.Logger.Error("CheckTitle: language_tag is required")
		return nil, status.Error(codes.InvalidArgument, "language_tag is required")
	}
	if req.SourceLanguage == "" {
		s.Logger.Error("CheckTitle: source_language is required")
		return nil, status.Error(codes.InvalidArgument, "source_language is required")
	}

	// Check translator health
	if s.Translator != nil {
		if err := s.Translator.CheckHealth(ctx); err != nil {
			s.Logger.WithError(err).Warn("Translator health check failed")
			return &nanabushv1.TitleCheckResponse{
				Ready:                false,
				Message:              fmt.Sprintf("Translator not ready: %v", err),
				EstimatedTimeSeconds: 0,
			}, nil
		}
	}

	// Estimate time based on title length (simple heuristic)
	// For lightweight MT, translation is typically faster than vLLM
	estimatedSeconds := int32(2 + len(req.Title)/20)
	if estimatedSeconds < 2 {
		estimatedSeconds = 2
	}
	if estimatedSeconds > 30 {
		estimatedSeconds = 30
	}

	s.Logger.WithFields(logrus.Fields{
		"ready":           true,
		"estimated_sec":   estimatedSeconds,
	}).Debug("CheckTitle response")

	return &nanabushv1.TitleCheckResponse{
		Ready:                true,
		Message:              "Ready to handle translation request",
		EstimatedTimeSeconds: estimatedSeconds,
	}, nil
}

// Translate performs full document translation.
// This is the main translation endpoint that processes complete documents.
func (s *TranslationService) Translate(ctx context.Context, req *nanabushv1.TranslateRequest) (*nanabushv1.TranslateResponse, error) {
	s.Logger.WithFields(logrus.Fields{
		"job_id":     req.JobId,
		"primitive":  req.Primitive,
		"namespace":  req.Namespace,
		"source_lang": req.SourceLanguage,
		"target_lang": req.TargetLanguage,
	}).Info("Translate request received")

	startTime := time.Now()

	// Validate request
	if req.JobId == "" {
		s.Logger.Error("Translate: job_id is required")
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}
	if req.TargetLanguage == "" {
		s.Logger.Error("Translate: target_language is required")
		return nil, status.Error(codes.InvalidArgument, "target_language is required")
	}
	if req.SourceLanguage == "" {
		s.Logger.Error("Translate: source_language is required")
		return nil, status.Error(codes.InvalidArgument, "source_language is required")
	}

	// Convert language codes to backend format
	sourceLang := s.LanguageMapper.ToBackendCode(req.SourceLanguage)
	targetLang := s.LanguageMapper.ToBackendCode(req.TargetLanguage)

	s.Logger.WithFields(logrus.Fields{
		"proto_source": req.SourceLanguage,
		"proto_target": req.TargetLanguage,
		"backend_source": sourceLang,
		"backend_target": targetLang,
	}).Debug("Language code conversion")

	var translatedTitle string
	var translatedMarkdown string
	var err error

	// Handle different primitive types
	switch req.Primitive {
	case nanabushv1.PrimitiveType_PRIMITIVE_TITLE:
		// Title-only translation
		if req.GetTitle() == "" {
			s.Logger.Error("Translate: title is required for PRIMITIVE_TITLE")
			return nil, status.Error(codes.InvalidArgument, "title is required for PRIMITIVE_TITLE")
		}

		if s.Translator != nil {
			translatedTitle, err = s.Translator.Translate(ctx, req.GetTitle(), sourceLang, targetLang)
			if err != nil {
				s.Logger.WithError(err).WithFields(logrus.Fields{
					"job_id": req.JobId,
				}).Error("Title translation failed")
				return &nanabushv1.TranslateResponse{
					JobId:        req.JobId,
					Success:      false,
					ErrorMessage: fmt.Sprintf("Translation failed: %v", err),
					CompletedAt:  timestamppb.Now(),
				}, nil
			}
		} else {
			s.Logger.Error("Translate: translator not configured")
			return &nanabushv1.TranslateResponse{
				JobId:        req.JobId,
				Success:      false,
				ErrorMessage: "Translator not configured",
				CompletedAt:  timestamppb.Now(),
			}, nil
		}

	case nanabushv1.PrimitiveType_PRIMITIVE_DOC_TRANSLATE:
		// Full document translation
		if req.GetDoc() == nil {
			s.Logger.Error("Translate: doc is required for PRIMITIVE_DOC_TRANSLATE")
			return nil, status.Error(codes.InvalidArgument, "doc is required for PRIMITIVE_DOC_TRANSLATE")
		}

		doc := req.GetDoc()
		s.Logger.WithFields(logrus.Fields{
			"job_id":        req.JobId,
			"title":         doc.Title,
			"markdown_len":  len(doc.Markdown),
		}).Debug("Translating document")

		if s.Translator != nil {
			// Translate title
			if doc.Title != "" {
				translatedTitle, err = s.Translator.Translate(ctx, doc.Title, sourceLang, targetLang)
				if err != nil {
					s.Logger.WithError(err).WithFields(logrus.Fields{
						"job_id": req.JobId,
					}).Error("Title translation failed")
					return &nanabushv1.TranslateResponse{
						JobId:        req.JobId,
						Success:      false,
						ErrorMessage: fmt.Sprintf("Title translation failed: %v", err),
						CompletedAt:  timestamppb.Now(),
					}, nil
				}
			}

			// Translate markdown content
			if doc.Markdown != "" {
				translatedMarkdown, err = s.Translator.Translate(ctx, doc.Markdown, sourceLang, targetLang)
				if err != nil {
					s.Logger.WithError(err).WithFields(logrus.Fields{
						"job_id": req.JobId,
					}).Error("Markdown translation failed")
					return &nanabushv1.TranslateResponse{
						JobId:        req.JobId,
						Success:      false,
						ErrorMessage: fmt.Sprintf("Markdown translation failed: %v", err),
						CompletedAt:  timestamppb.Now(),
					}, nil
				}
			}
		} else {
			s.Logger.Error("Translate: translator not configured")
			return &nanabushv1.TranslateResponse{
				JobId:        req.JobId,
				Success:      false,
				ErrorMessage: "Translator not configured",
				CompletedAt:  timestamppb.Now(),
			}, nil
		}

	default:
		s.Logger.WithFields(logrus.Fields{
			"primitive": req.Primitive,
		}).Error("Unsupported primitive type")
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("unsupported primitive type: %v", req.Primitive))
	}

	// Build response
	inferenceTime := time.Since(startTime).Seconds()

	s.Logger.WithFields(logrus.Fields{
		"job_id":         req.JobId,
		"success":        true,
		"inference_time": inferenceTime,
	}).Info("Translation completed successfully")

	resp := &nanabushv1.TranslateResponse{
		JobId:               req.JobId,
		Success:             true,
		CompletedAt:         timestamppb.Now(),
		TokensUsed:          0, // Lightweight MT doesn't use tokens
		InferenceTimeSeconds: inferenceTime,
	}

	if translatedTitle != "" {
		resp.TranslatedTitle = translatedTitle
	}
	if translatedMarkdown != "" {
		resp.TranslatedMarkdown = translatedMarkdown
	}

	return resp, nil
}

// TranslateStream supports streaming for large documents.
// Client sends chunks, server responds with translated chunks.
// Note: This is a simplified implementation. For production, consider
// implementing proper chunking and streaming translation.
func (s *TranslationService) TranslateStream(stream nanabushv1.TranslationService_TranslateStreamServer) error {
	s.Logger.Info("TranslateStream request started")

	var jobID string
	chunkIndex := int32(0)

	for {
		// Receive chunk from client
		chunk, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				// Client closed stream
				s.Logger.WithFields(logrus.Fields{
					"job_id": jobID,
				}).Debug("TranslateStream: client closed stream")
				break
			}
			s.Logger.WithError(err).Error("TranslateStream receive error")
			return status.Error(codes.Internal, fmt.Sprintf("failed to receive chunk: %v", err))
		}

		if jobID == "" {
			jobID = chunk.JobId
			s.Logger.WithFields(logrus.Fields{
				"job_id": jobID,
			}).Info("TranslateStream started")
		}

		// Check if this is the final chunk
		if chunk.IsFinal {
			s.Logger.WithFields(logrus.Fields{
				"job_id": jobID,
			}).Debug("TranslateStream final chunk received")

			// Send final acknowledgment
			if err := stream.Send(&nanabushv1.TranslateChunk{
				JobId:      jobID,
				ChunkIndex: chunkIndex,
				IsFinal:    true,
				Content:    "[Stream completed]",
			}); err != nil {
				s.Logger.WithError(err).Error("TranslateStream: failed to send final chunk")
				return status.Error(codes.Internal, fmt.Sprintf("failed to send final chunk: %v", err))
			}
			break
		}

		// TODO: Implement actual streaming translation
		// For now, echo back with translation placeholder
		// In production, this should translate each chunk and send it back
		translatedContent := chunk.Content + " [translated chunk " + fmt.Sprintf("%d", chunkIndex) + "]"

		// Send translated chunk back to client
		if err := stream.Send(&nanabushv1.TranslateChunk{
			JobId:      jobID,
			ChunkIndex: chunkIndex,
			IsFinal:    false,
			Content:    translatedContent,
		}); err != nil {
			s.Logger.WithError(err).Error("TranslateStream send error")
			return status.Error(codes.Internal, fmt.Sprintf("failed to send chunk: %v", err))
		}

		chunkIndex++
	}

	s.Logger.WithFields(logrus.Fields{
		"job_id": jobID,
	}).Info("TranslateStream completed")

	return nil
}

// GetRegisteredClients returns all currently registered clients (for monitoring/debugging).
func (s *TranslationService) GetRegisteredClients() []*ClientInfo {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	clients := make([]*ClientInfo, 0, len(s.clients))
	for _, client := range s.clients {
		// Create a copy to avoid race conditions
		clientCopy := *client
		clients = append(clients, &clientCopy)
	}
	return clients
}

// CleanupExpiredClients removes clients that haven't sent a heartbeat in a while.
// This should be called periodically (e.g., every 5 minutes).
func (s *TranslationService) CleanupExpiredClients(maxIdleTime time.Duration) {
	s.clientsMutex.Lock()
	defer s.clientsMutex.Unlock()

	now := time.Now()
	removed := 0

	for clientID, client := range s.clients {
		if now.Sub(client.LastHeartbeat) > maxIdleTime {
			s.Logger.WithFields(logrus.Fields{
				"client_id":      clientID,
				"client_name":    client.ClientName,
				"last_heartbeat": client.LastHeartbeat,
			}).Info("Removing expired client")
			delete(s.clients, clientID)
			removed++
		}
	}

	if removed > 0 {
		s.Logger.WithFields(logrus.Fields{
			"removed":   removed,
			"remaining": len(s.clients),
		}).Info("Cleaned up expired clients")
	}
}

