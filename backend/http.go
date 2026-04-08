package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

const sessionTTL = 24 * time.Hour

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

type webhookPayload struct {
	DocumentID *int64 `json:"document_id,omitempty"`
	ID         *int64 `json:"id,omitempty"`
	Document   *struct {
		ID    *int64 `json:"id,omitempty"`
		Title string `json:"title,omitempty"`
	} `json:"document,omitempty"`
	Title string `json:"title,omitempty"`
}

type documentTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type paperlessTagsResponse struct {
	Count   int           `json:"count"`
	Results []documentTag `json:"results"`
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
	api.POST("/auth/login", s.handleLogin)
	api.POST("/webhooks/paperless", s.handlePaperlessWebhook)

	authenticated := api.Group("")
	authenticated.Use(s.authMiddleware())
	authenticated.GET("/auth/me", s.handleMe)
	authenticated.POST("/auth/logout", s.handleLogout)
	authenticated.GET("/config", s.handleGetConfig)
	authenticated.PUT("/config", s.handlePutConfig)
	authenticated.GET("/queue", s.handleListQueue)
	authenticated.POST("/queue/:id/process", s.handleProcessQueueItem)
	authenticated.GET("/dashboard", s.handleDashboard)
	authenticated.GET("/models", s.handleListModels)
	authenticated.GET("/paperless/tags", s.handleListPaperlessTags)

	return router
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

func (s *Server) handleProcessQueueItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queue item id must be numeric"})
		return
	}

	item, err := s.processor.ProcessByID(c.Request.Context(), id)
	if errors.Is(err, errQueueItemNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue item not found"})
		return
	}
	if errors.Is(err, errQueueItemNotPending) {
		c.JSON(http.StatusConflict, gin.H{"error": "queue item is not pending"})
		return
	}
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, item)
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

func (s *Server) handlePaperlessWebhook(c *gin.Context) {
	if s.sharedSecret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook shared secret is not configured"})
		return
	}

	if c.GetHeader("x-shared-secret") != s.sharedSecret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid shared secret"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read webhook body"})
		return
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}

	documentID := extractDocumentID(payload)
	if documentID != nil {
		existing, err := s.store.FindActiveQueueItemByDocumentID(c.Request.Context(), *documentID)
		if err != nil {
			s.writeInternalError(c, err)
			return
		}
		if existing != nil {
			c.JSON(http.StatusAccepted, gin.H{"item": existing, "reused": true})
			return
		}
	}

	title := extractDocumentTitle(payload)
	if title == "" {
		title = "Untitled document"
	}

	item, err := s.store.CreateQueueItem(c.Request.Context(), documentID, title, "paperless", "webhook", string(body))
	if err != nil {
		s.writeInternalError(c, err)
		return
	}

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
		if len(c.Errors) > 0 || c.Writer.Status() >= http.StatusBadRequest {
			event = s.logger.Error()
		}

		event.
			Str("request_id", fmt.Sprint(requestID)).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(startedAt)).
			Str("client_ip", c.ClientIP()).
			Msg("http request completed")
	}
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

func extractDocumentID(payload webhookPayload) *int64 {
	if payload.DocumentID != nil {
		return payload.DocumentID
	}
	if payload.ID != nil {
		return payload.ID
	}
	if payload.Document != nil && payload.Document.ID != nil {
		return payload.Document.ID
	}

	return nil
}

func extractDocumentTitle(payload webhookPayload) string {
	if payload.Document != nil && strings.TrimSpace(payload.Document.Title) != "" {
		return strings.TrimSpace(payload.Document.Title)
	}

	return strings.TrimSpace(payload.Title)
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}

	return hex.EncodeToString(buffer)
}
