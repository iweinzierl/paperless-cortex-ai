package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func main() {
	logger := newLogger()
	enableOutgoingHTTPLogging(logger)
	
	listenAddr := envOrDefault("PAPERLESS_AIEXT_LISTEN_ADDR", ":8080")
	databasePath := envOrDefault("PAPERLESS_AIEXT_DB_PATH", "backend/data/paperless-aiext.db")

	fmt.Fprintf(os.Stderr, "paperless-ai-ext backend starting on %s using %s\n", listenAddr, databasePath)

	store, err := OpenStore(databasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "paperless-ai-ext backend failed to open store: %v\n", err)
		logger.Fatal().Err(err).Msg("failed to open backend store")
	}
	defer store.Close()

	processor := NewProcessor(store, logger)
	server := NewServer(store, processor, logger, os.Getenv("PAPERLESS_AIEXT_SHARED_SECRET"))
	router := server.Router()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	processor.Start(ctx)

	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("backend shutdown failed")
		}
	}()

	fmt.Fprintf(os.Stderr, "paperless-ai-ext backend listening on %s\n", httpServer.Addr)
	logger.Info().Str("addr", httpServer.Addr).Msg("starting backend server")
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "paperless-ai-ext backend stopped unexpectedly: %v\n", err)
		logger.Fatal().Err(err).Msg("backend server stopped unexpectedly")
	}
	fmt.Fprintln(os.Stderr, "paperless-ai-ext backend stopped")
	logger.Info().Msg("backend server stopped")
}

func newLogger() zerolog.Logger {
	level := zerolog.InfoLevel
	if rawLevel := strings.ToLower(strings.TrimSpace(os.Getenv("PAPERLESS_AIEXT_LOGLEVEL"))); rawLevel != "" {
		if parsedLevel, err := zerolog.ParseLevel(rawLevel); err == nil {
			level = parsedLevel
		}
	}

	zerolog.SetGlobalLevel(level)
	gin.SetMode(gin.ReleaseMode)
	
	var writer io.Writer = os.Stderr
	if strings.ToLower(strings.TrimSpace(os.Getenv("PAPERLESS_AIEXT_PRETTY_LOGS"))) == "true" {
		writer = zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	}

	return zerolog.New(writer).With().Timestamp().Logger().Level(level)
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func nowMS() int64 {
	return time.Now().UnixMilli()
}
