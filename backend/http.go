package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"paperless-ai-ext/internal/paperless"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

const sessionTTL = 24 * time.Hour

var errWebhookUnsupportedMediaType = errors.New("unsupported webhook content type")
var errWebhookInvalidPayload = errors.New("invalid webhook payload")
var errWebhookMissingDocumentURL = errors.New("webhook payload does not include a document url")
var errWebhookInvalidDocumentURL = errors.New("webhook payload includes an invalid document url")

type Server struct {
	store        *Store
	processor    *Processor
	logger       zerolog.Logger
	sharedSecret string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
}

type ollamaModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type ollamaModel struct {
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	ModifiedAt string             `json:"modified_at"`
	Size       int64              `json:"size"`
	Digest     string             `json:"digest"`
	Details    ollamaModelDetails `json:"details"`
}

type ollamaModelsResponse struct {
	Models []ollamaModel `json:"models"`
}

type paperlessWebhookRequest struct {
	ContentType   string
	DocumentID    *int64
	DocumentTitle string
	DocumentURL   string
	Trigger       string
	StoredPayload string
	BodySize      int
	BodyPreview   string
}

type webhookValidationError struct {
	cause         error
	detail        string
	contentType   string
	contentLength int64
	bodySize      int
	bodyPreview   string
	documentURL   string
	documentID    *int64
}

type storedWebhookPayload struct {
	ContentType   string         `json:"content_type"`
	DocumentID    *int64         `json:"document_id,omitempty"`
	DocumentTitle string         `json:"document_title,omitempty"`
	DocumentURL   string         `json:"document_url,omitempty"`
	JSON          map[string]any `json:"json,omitempty"`
}

type paperlessWebhookPayload struct {
	DocumentTitle string `json:"document_title"`
	DocumentURL   string `json:"document_url"`
}

type documentTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type paperlessTagsResponse struct {
	Count   int           `json:"count"`
	Results []documentTag `json:"results"`
}

type dependencyStatusResponse struct {
	Configured bool   `json:"configured"`
	Healthy    bool   `json:"healthy"`
	Message    string `json:"message"`
	ModelCount int    `json:"model_count,omitempty"`
}

type systemStatusResponse struct {
	Backend   dependencyStatusResponse `json:"backend"`
	Paperless dependencyStatusResponse `json:"paperless"`
	Ollama    dependencyStatusResponse `json:"ollama"`
}

type documentProcessHistoryResponse struct {
	DocumentID    int64       `json:"document_id"`
	DocumentTitle string      `json:"document_title"`
	Items         []QueueItem `json:"items"`
}

func NewServer(store *Store, processor *Processor, logger zerolog.Logger, sharedSecret string) *Server {
	return &Server{
		store:        store,
		processor:    processor,
		logger:       logger,
		sharedSecret: strings.TrimSpace(sharedSecret),
	}
}

func (s *Server) Router() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestIDMiddleware())
	router.Use(s.loggingMiddleware())

	api := router.Group("/api")
	api.GET("/health", s.handleHealth)
	api.GET("/status", s.handleStatus)
	api.POST("/auth/login", s.handleLogin)
	api.POST("/webhooks/paperless", s.handlePaperlessWebhook)

	authenticated := api.Group("")
	authenticated.Use(s.authMiddleware())
	authenticated.GET("/auth/me", s.handleMe)
	authenticated.POST("/auth/logout", s.handleLogout)
	authenticated.GET("/config", s.handleGetConfig)
	authenticated.PUT("/config", s.handlePutConfig)
	authenticated.GET("/queue", s.handleListQueue)
	authenticated.GET("/documents/:id/processes", s.handleDocumentProcessHistory)
	authenticated.DELETE("/queue/:id", s.handleDeleteQueueItem)
	authenticated.POST("/queue/:id/process", s.handleProcessQueueItem)
	authenticated.POST("/queue/:id/apply", s.handleApplyQueueItem)
	authenticated.GET("/dashboard", s.handleDashboard)
	authenticated.GET("/models", s.handleListModels)
	authenticated.GET("/paperless/tags", s.handleListPaperlessTags)

	return router
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleStatus(c *gin.Context) {
	cfg, err := s.store.LoadConfig(c.Request.Context())
	if err != nil {
		s.writeInternalError(c, err)
		return
	}
	cfg.Normalize()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	c.JSON(http.StatusOK, systemStatusResponse{
		Backend: dependencyStatusResponse{
			Configured: true,
			Healthy:    true,
			Message:    "Backend online",
		},
		Paperless: s.paperlessDependencyStatus(ctx, cfg),
		Ollama:    s.ollamaDependencyStatus(ctx, cfg),
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
		return
	}

	request.Username = strings.TrimSpace(request.Username)
	if request.Username == "" || request.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	cfg, err := s.store.LoadConfig(c.Request.Context())
	if err != nil {
		s.writeInternalError(c, err)
		return
	}
	if cfg.Paperless.PaperlessURL == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "paperless_url must be configured before login"})
		return
	}

	sessionToken, err := authenticateAgainstPaperless(c.Request.Context(), cfg.Paperless.PaperlessURL, request.Username, request.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	now := nowMS()
	session := Session{
		Token:        sessionToken,
		Username:     request.Username,
		CreatedAtMS:  now,
		ExpiresAtMS:  now + sessionTTL.Milliseconds(),
		LastSeenAtMS: now,
	}
	if err := s.store.CreateSession(c.Request.Context(), session); err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Token:       session.Token,
		Username:    session.Username,
		ExpiresAtMS: session.ExpiresAtMS,
	})
}

func (s *Server) handleMe(c *gin.Context) {
	session := mustSession(c)
	c.JSON(http.StatusOK, session)
}

func (s *Server) handleLogout(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if err := s.store.DeleteSession(c.Request.Context(), token); err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) handleGetConfig(c *gin.Context) {
	cfg, err := s.store.LoadConfig(c.Request.Context())
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, cfg)
}

func (s *Server) handlePutConfig(c *gin.Context) {
	var cfg BackendConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backend config"})
		return
	}

	cfg.Normalize()
	if err := s.store.SaveConfig(c.Request.Context(), cfg); err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, cfg)
}

func (s *Server) handleListQueue(c *gin.Context) {
	limit := 50
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a number"})
			return
		}
		limit = parsedLimit
	}

	items, err := s.store.ListQueueItems(c.Request.Context(), strings.TrimSpace(c.Query("status")), limit)
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) handleDocumentProcessHistory(c *gin.Context) {
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document id must be numeric"})
		return
	}

	limit := 100
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a number"})
			return
		}
		limit = parsedLimit
	}

	history, err := s.store.GetDocumentProcessHistory(c.Request.Context(), documentID, limit)
	if errors.Is(err, errQueueItemNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "document process history not found"})
		return
	}
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, documentProcessHistoryResponse{
		DocumentID:    history.DocumentID,
		DocumentTitle: history.DocumentTitle,
		Items:         history.Items,
	})
}

func (s *Server) handleProcessQueueItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queue item id must be numeric"})
		return
	}

	item, err := s.processor.StartProcessByID(c.Request.Context(), id)
	if errors.Is(err, errQueueItemNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if errors.Is(err, errQueueItemNotRetryable) {
		c.JSON(http.StatusConflict, gin.H{"error": "queue item cannot be retriggered unless it is pending, failed, or partially completed"})
		return
	}
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, item)
}

func (s *Server) handleApplyQueueItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queue item id must be numeric"})
		return
	}

	item, err := s.processor.ApplyByID(c.Request.Context(), id)
	if errors.Is(err, errQueueItemNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if errors.Is(err, errQueueItemAlreadyApplied) {
		c.JSON(http.StatusConflict, gin.H{"error": "queue item suggestions were already applied"})
		return
	}
	if errors.Is(err, errQueueItemNotApplyable) {
		c.JSON(http.StatusConflict, gin.H{"error": "queue item does not have applyable suggestions"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, item)
}

func (s *Server) handleDeleteQueueItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queue item id must be numeric"})
		return
	}

	err = s.store.DeleteQueueItem(c.Request.Context(), id)
	if errors.Is(err, errQueueItemNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if errors.Is(err, errQueueItemNotRemovable) {
		c.JSON(http.StatusConflict, gin.H{"error": "queue item cannot be removed while it is processing"})
		return
	}
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) handleDashboard(c *gin.Context) {
	stats, err := s.store.BuildDashboardStats(c.Request.Context(), 20)
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleListModels(c *gin.Context) {
	cfg, err := s.store.LoadConfig(c.Request.Context())
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	if cfg.LLMs.OllamaURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ollama is not configured"})
		return
	}

	tagsURL := fmt.Sprintf("%s/api/tags", strings.TrimRight(cfg.LLMs.OllamaURL, "/"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to ollama API"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("ollama API returned status %s", resp.Status)})
		return
	}

	var payload ollamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to decode ollama API response"})
		return
	}

	c.JSON(http.StatusOK, payload)
}

func (s *Server) handleListPaperlessTags(c *gin.Context) {
	cfg, err := s.store.LoadConfig(c.Request.Context())
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "paperless is not fully configured"})
		return
	}

	tagsURL := fmt.Sprintf("%s/api/tags/", strings.TrimRight(cfg.Paperless.PaperlessURL, "/"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		s.writeInternalError(c, err)
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", cfg.Paperless.PaperlessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to paperless API"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("paperless API returned status %s", resp.Status)})
		return
	}

	var payload paperlessTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to decode paperless API response"})
		return
	}

	c.JSON(http.StatusOK, payload.Results)
}

func (s *Server) paperlessDependencyStatus(ctx context.Context, cfg BackendConfig) dependencyStatusResponse {
	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		return dependencyStatusResponse{
			Configured: false,
			Healthy:    false,
			Message:    "Paperless not configured",
		}
	}

	tagsURL := fmt.Sprintf("%s/api/tags/", strings.TrimRight(cfg.Paperless.PaperlessURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    "Paperless request failed",
		}
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", cfg.Paperless.PaperlessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    "Paperless unreachable",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    fmt.Sprintf("Paperless returned %s", resp.Status),
		}
	}

	return dependencyStatusResponse{
		Configured: true,
		Healthy:    true,
		Message:    "Paperless connected",
	}
}

func (s *Server) ollamaDependencyStatus(ctx context.Context, cfg BackendConfig) dependencyStatusResponse {
	if cfg.LLMs.OllamaURL == "" {
		return dependencyStatusResponse{
			Configured: false,
			Healthy:    false,
			Message:    "Ollama not configured",
		}
	}

	tagsURL := fmt.Sprintf("%s/api/tags", strings.TrimRight(cfg.LLMs.OllamaURL, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    "Ollama request failed",
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    "Ollama unreachable",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    fmt.Sprintf("Ollama returned %s", resp.Status),
		}
	}

	var payload ollamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return dependencyStatusResponse{
			Configured: true,
			Healthy:    false,
			Message:    "Ollama response invalid",
		}
	}

	return dependencyStatusResponse{
		Configured: true,
		Healthy:    true,
		Message:    "Ollama connected",
		ModelCount: len(payload.Models),
	}
}

func (s *Server) handlePaperlessWebhook(c *gin.Context) {
	requestLogger := s.requestLogger(c).With().
		Str("client_ip", c.ClientIP()).
		Str("webhook_content_type", normalizedContentType(c.GetHeader("Content-Type"))).
		Int64("content_length", c.Request.ContentLength).
		Bool("has_shared_secret_header", strings.TrimSpace(c.GetHeader("x-shared-secret")) != "").
		Logger()

	if s.sharedSecret == "" {
		requestLogger.Error().Msg("paperless webhook rejected: shared secret is not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook shared secret is not configured"})
		return
	}

	if subtle.ConstantTimeCompare([]byte(c.GetHeader("x-shared-secret")), []byte(s.sharedSecret)) != 1 {
		requestLogger.Warn().Msg("paperless webhook rejected: invalid shared secret")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid shared secret"})
		return
	}

	webhookRequest, err := parsePaperlessWebhookRequest(c)
	if errors.Is(err, errWebhookUnsupportedMediaType) {
		s.logWebhookValidationFailure(requestLogger, err)
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "unsupported webhook content type; use application/json",
		})
		return
	}
	if errors.Is(err, errWebhookMissingDocumentURL) {
		s.logWebhookValidationFailure(requestLogger, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "webhook payload must include document_url",
		})
		return
	}
	if errors.Is(err, errWebhookInvalidDocumentURL) {
		s.logWebhookValidationFailure(requestLogger, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "webhook payload must include a document_url with an embedded numeric document ID",
		})
		return
	}
	if errors.Is(err, errWebhookInvalidPayload) {
		s.logWebhookValidationFailure(requestLogger, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}
	if err != nil {
		requestLogger.Error().Err(err).Msg("paperless webhook failed unexpectedly")
		s.writeInternalError(c, err)
		return
	}

	logEvent := requestLogger.Info().
		Str("webhook_content_type", webhookRequest.ContentType).
		Int("body_size", webhookRequest.BodySize).
		Str("document_url", webhookRequest.DocumentURL)
	if webhookRequest.BodyPreview != "" {
		logEvent = logEvent.Str("body_preview", webhookRequest.BodyPreview)
	}
	if webhookRequest.DocumentID != nil {
		logEvent = logEvent.Int64("document_id", *webhookRequest.DocumentID)
	}

	if webhookRequest.DocumentID != nil {
		existing, err := s.store.FindActiveQueueItemByDocumentID(c.Request.Context(), *webhookRequest.DocumentID)
		if err != nil {
			s.writeInternalError(c, err)
			return
		}
		if existing != nil {
			logEvent.Bool("reused", true).Int64("queue_item_id", existing.ID).Msg("reused queued paperless webhook")
			c.JSON(http.StatusAccepted, gin.H{"item": existing, "reused": true})
			return
		}
	}

	title := webhookRequest.DocumentTitle
	if title == "" {
		title = "Untitled document"
	}

	requestedStages := []string(nil)
	if cfg, err := s.store.LoadConfig(c.Request.Context()); err != nil {
		s.writeInternalError(c, err)
		return
	} else {
		requestedStages, err = resolveRequestedStagesForQueueItem(c.Request.Context(), cfg, webhookRequest.DocumentID)
		if err != nil {
			requestLogger.Warn().Err(err).Msg("failed to resolve requested stages for queued paperless webhook")
			requestedStages = nil
		}
	}

	item, err := s.store.CreateQueueItemWithRequestedStages(
		c.Request.Context(),
		webhookRequest.DocumentID,
		title,
		"paperless",
		webhookRequest.Trigger,
		webhookRequest.StoredPayload,
		requestedStages,
	)
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	logEvent.Bool("reused", false).Int64("queue_item_id", item.ID).Msg("queued paperless webhook")
	c.JSON(http.StatusAccepted, gin.H{"item": item, "reused": false})
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		session, err := s.store.GetSession(c.Request.Context(), token)
		if err != nil {
			s.writeAbortInternalError(c, err)
			return
		}
		if session == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			return
		}

		if err := s.store.TouchSession(c.Request.Context(), token); err != nil {
			s.writeAbortInternalError(c, err)
			return
		}

		c.Set("session", *session)
		c.Next()
	}
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		requestID, _ := c.Get("request_id")
		event := s.logger.Info()
		switch {
		case len(c.Errors) > 0 || c.Writer.Status() >= http.StatusInternalServerError:
			event = s.logger.Error()
		case c.Writer.Status() >= http.StatusBadRequest:
			event = s.logger.Warn()
		}

		event = event.
			Str("request_id", fmt.Sprint(requestID)).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(startedAt)).
			Str("client_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Int64("content_length", c.Request.ContentLength)
		if rawQuery := strings.TrimSpace(c.Request.URL.RawQuery); rawQuery != "" {
			event = event.Str("query", rawQuery)
		}
		if contentType := normalizedContentType(c.GetHeader("Content-Type")); contentType != "" {
			event = event.Str("content_type", contentType)
		}
		if len(c.Errors) > 0 {
			event = event.Strs("errors", c.Errors.Errors())
		}

		event.Msg("http request completed")
	}
}

func (s *Server) requestLogger(c *gin.Context) zerolog.Logger {
	requestID, _ := c.Get("request_id")

	return s.logger.With().
		Str("request_id", fmt.Sprint(requestID)).
		Str("method", c.Request.Method).
		Str("path", c.Request.URL.Path).
		Logger()
}

func (s *Server) logWebhookValidationFailure(logger zerolog.Logger, err error) {
	event := logger.Warn().Err(err)
	var validationErr *webhookValidationError
	if errors.As(err, &validationErr) {
		if validationErr.contentType != "" {
			event = event.Str("webhook_content_type", validationErr.contentType)
		}
		if validationErr.contentLength >= 0 {
			event = event.Int64("content_length", validationErr.contentLength)
		}
		if validationErr.bodySize > 0 {
			event = event.Int("body_size", validationErr.bodySize)
		}
		if validationErr.bodyPreview != "" {
			event = event.Str("body_preview", validationErr.bodyPreview)
		}
		if validationErr.documentURL != "" {
			event = event.Str("document_url", validationErr.documentURL)
		}
		if validationErr.documentID != nil {
			event = event.Int64("document_id", *validationErr.documentID)
		}
		if validationErr.detail != "" {
			event = event.Str("reason", validationErr.detail)
		}
	}

	event.Msg("paperless webhook rejected")
}

func resolveRequestedStagesForQueueItem(ctx context.Context, cfg BackendConfig, documentID *int64) ([]string, error) {
	if documentID == nil {
		return nil, nil
	}
	if cfg.Paperless.PaperlessURL == "" || cfg.Paperless.PaperlessToken == "" {
		return nil, nil
	}

	client := paperless.NewClient(cfg.Paperless.PaperlessURL, cfg.Paperless.PaperlessToken)
	tags, err := client.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list paperless tags for queue plan: %w", err)
	}
	document, err := client.GetDocument(ctx, *documentID)
	if err != nil {
		return nil, fmt.Errorf("load paperless document for queue plan: %w", err)
	}

	tagNameSet, _ := buildTagNameSet(tags, document.TagIDs)
	plan := buildProcessingPlan(cfg.Process, tagNameSet)
	return append([]string(nil), plan.RequestedStageList...), nil
}

func (s *Server) writeInternalError(c *gin.Context, err error) {
	s.logger.Error().Err(err).Msg("request failed")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func (s *Server) writeAbortInternalError(c *gin.Context, err error) {
	s.logger.Error().Err(err).Msg("request failed")
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func authenticateAgainstPaperless(parent context.Context, paperlessURL string, username string, password string) (string, error) {
	authURL, err := buildPaperlessAuthURL(paperlessURL)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(gin.H{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", fmt.Errorf("marshal login payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build paperless auth request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("call paperless auth API: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read paperless auth response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("paperless authentication failed with %s", response.Status)
	}

	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return "", fmt.Errorf("decode paperless auth response: %w", err)
	}

	for _, key := range []string{"token", "auth_token", "key"} {
		if token, ok := payload[key].(string); ok && strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}

	return "", errors.New("paperless authentication response did not include a session token")
}

func buildPaperlessAuthURL(rawBaseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil {
		return "", fmt.Errorf("parse paperless URL: %w", err)
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/api/token/"
	case strings.HasSuffix(path, "/api/token"):
		parsed.Path = path + "/"
	case strings.HasSuffix(path, "/api"):
		parsed.Path = path + "/token/"
	default:
		parsed.Path = path + "/api/token/"
	}

	return parsed.String(), nil
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := randomHex(8)
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Allow-Methods", "*")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func mustSession(c *gin.Context) Session {
	rawSession, _ := c.Get("session")
	session, _ := rawSession.(Session)
	return session
}

func parsePaperlessWebhookRequest(c *gin.Context) (*paperlessWebhookRequest, error) {
	contentType := normalizedContentType(c.GetHeader("Content-Type"))
	contentLength := c.Request.ContentLength

	switch contentType {
	case "", "application/json", "text/json":
		request, err := parseJSONWebhookRequest(c.Request, contentType, contentLength)
		if err != nil {
			return nil, err
		}
		return request, nil
	default:
		trimmedType := strings.TrimSpace(contentType)
		if trimmedType == "" {
			return parseJSONWebhookRequest(c.Request, "application/json", contentLength)
		}
		return nil, &webhookValidationError{
			cause:         errWebhookUnsupportedMediaType,
			detail:        fmt.Sprintf("unsupported content type %q", trimmedType),
			contentType:   trimmedType,
			contentLength: contentLength,
		}
	}
}

func parseJSONWebhookRequest(request *http.Request, contentType string, contentLength int64) (*paperlessWebhookRequest, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("read webhook body: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, &webhookValidationError{
			cause:         errWebhookInvalidPayload,
			detail:        "request body is empty",
			contentType:   contentType,
			contentLength: contentLength,
			bodySize:      len(body),
		}
	}

	normalizedBody, err := normalizeJSONStringWebhookPayload(body)
	if err != nil {
		return nil, &webhookValidationError{
			cause:         errWebhookInvalidPayload,
			detail:        fmt.Sprintf("json string payload decode failed: %v", err),
			contentType:   contentType,
			contentLength: contentLength,
			bodySize:      len(body),
			bodyPreview:   summarizeWebhookBody(body),
		}
	}

	bodyPreview := summarizeWebhookBody(normalizedBody)

	decoder := json.NewDecoder(bytes.NewReader(normalizedBody))
	decoder.UseNumber()

	var payload paperlessWebhookPayload
	if err := decoder.Decode(&payload); err != nil {
		return nil, &webhookValidationError{
			cause:         errWebhookInvalidPayload,
			detail:        fmt.Sprintf("json decode failed: %v", err),
			contentType:   contentType,
			contentLength: contentLength,
			bodySize:      len(body),
			bodyPreview:   bodyPreview,
		}
	}
	if decoder.More() {
		return nil, &webhookValidationError{
			cause:         errWebhookInvalidPayload,
			detail:        "json payload contains multiple top-level values",
			contentType:   contentType,
			contentLength: contentLength,
			bodySize:      len(body),
			bodyPreview:   bodyPreview,
		}
	}

	documentURL := strings.TrimSpace(payload.DocumentURL)
	if documentURL == "" {
		return nil, &webhookValidationError{
			cause:         errWebhookMissingDocumentURL,
			detail:        "document_url is missing or empty",
			contentType:   contentType,
			contentLength: contentLength,
			bodySize:      len(body),
			bodyPreview:   bodyPreview,
		}
	}

	documentID, err := extractDocumentIDFromURL(documentURL)
	if err != nil {
		if errors.Is(err, errWebhookInvalidDocumentURL) {
			return nil, &webhookValidationError{
				cause:         errWebhookInvalidDocumentURL,
				detail:        fmt.Sprintf("document_url %q does not include an embedded numeric document id", documentURL),
				contentType:   contentType,
				contentLength: contentLength,
				bodySize:      len(body),
				bodyPreview:   bodyPreview,
				documentURL:   documentURL,
			}
		}
		return nil, err
	}

	storedPayload, err := json.Marshal(storedWebhookPayload{
		ContentType:   contentType,
		DocumentID:    documentID,
		DocumentTitle: strings.TrimSpace(payload.DocumentTitle),
		DocumentURL:   documentURL,
		JSON: map[string]any{
			"document_title": strings.TrimSpace(payload.DocumentTitle),
			"document_url":   documentURL,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stored webhook payload: %w", err)
	}

	return &paperlessWebhookRequest{
		ContentType:   contentType,
		DocumentID:    documentID,
		DocumentTitle: strings.TrimSpace(payload.DocumentTitle),
		DocumentURL:   documentURL,
		Trigger:       "webhook",
		StoredPayload: string(storedPayload),
		BodySize:      len(body),
		BodyPreview:   bodyPreview,
	}, nil
}

func (e *webhookValidationError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.detail) != "" {
		return e.detail
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return "webhook validation failed"
}

func normalizeJSONStringWebhookPayload(body []byte) ([]byte, error) {
	trimmedBody := bytes.TrimSpace(body)
	if len(trimmedBody) == 0 || trimmedBody[0] != '"' {
		return body, nil
	}

	var payload string
	if err := json.Unmarshal(trimmedBody, &payload); err != nil {
		return nil, err
	}

	decoded := strings.TrimSpace(payload)
	if decoded == "" {
		return body, nil
	}

	return []byte(decoded), nil
}

func (e *webhookValidationError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}

func summarizeWebhookBody(body []byte) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(string(body))), " ")
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 512 {
		return trimmed
	}

	return trimmed[:512] + "..."
}

func extractDocumentIDFromURL(rawDocumentURL string) (*int64, error) {
	trimmedURL := strings.TrimSpace(rawDocumentURL)
	if trimmedURL == "" {
		return nil, errWebhookMissingDocumentURL
	}

	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		return nil, errWebhookInvalidDocumentURL
	}

	for _, candidate := range []string{parsedURL.Path, parsedURL.Fragment} {
		if parsedID := extractDocumentIDFromPathLike(candidate); parsedID != nil {
			return parsedID, nil
		}
	}

	return nil, errWebhookInvalidDocumentURL
}

func extractDocumentIDFromPathLike(rawPath string) *int64 {
	replacer := strings.NewReplacer("#", "/", "?", "/", "&", "/", "=", "/")
	segments := strings.Split(replacer.Replace(strings.TrimSpace(rawPath)), "/")
	for index, segment := range segments {
		if strings.TrimSpace(segment) != "documents" {
			continue
		}
		for nextIndex := index + 1; nextIndex < len(segments); nextIndex++ {
			trimmed := strings.TrimSpace(segments[nextIndex])
			if trimmed == "" {
				continue
			}
			parsedID, err := strconv.ParseInt(trimmed, 10, 64)
			if err == nil {
				return &parsedID
			}
			break
		}
	}

	return nil
}

func normalizedContentType(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return strings.ToLower(trimmed)
	}

	return strings.ToLower(mediaType)
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}

	return hex.EncodeToString(buffer)
}
