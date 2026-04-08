package main

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

const maxLoggedHTTPBodyBytes = 4096

type loggingRoundTripper struct {
	next   http.RoundTripper
	logger zerolog.Logger
}

func (rt *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startedAt := time.Now()

	resp, err := rt.next.RoundTrip(req)

	latency := time.Since(startedAt)

	event := rt.logger.Info()
	if err != nil {
		event = rt.logger.Error().Err(err)
	} else if resp != nil && resp.StatusCode >= 400 {
		event = rt.logger.Warn() // Warn or Error, backend server uses Error for 4xx as well
	}

	event.
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Dur("latency", latency)

	if resp != nil {
		event.Int("status", resp.StatusCode)
		if contentType := normalizedContentType(resp.Header.Get("Content-Type")); contentType != "" {
			event = event.Str("response_content_type", contentType)
		}
		if resp.ContentLength >= 0 {
			event = event.Int64("response_content_length", resp.ContentLength)
		}
		if resp.StatusCode >= 400 {
			bodyPreview, bodySize, truncated, readErr := readAndRestoreBodyPreview(resp)
			if readErr != nil {
				event = event.Str("response_body_read_error", readErr.Error())
			} else {
				if bodySize >= 0 {
					event = event.Int("response_body_size", bodySize)
				}
				if bodyPreview != "" {
					event = event.Str("response_body_preview", bodyPreview)
				}
				event = event.Bool("response_body_truncated", truncated)
			}
		}
	}

	event.Msg("outgoing http request")

	return resp, err
}

func readAndRestoreBodyPreview(resp *http.Response) (preview string, size int, truncated bool, err error) {
	if resp == nil || resp.Body == nil {
		return "", 0, false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", -1, false, err
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	preview, truncated = summarizeHTTPBody(body)
	return preview, len(body), truncated, nil
}

func summarizeHTTPBody(body []byte) (string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", false
	}

	truncated := len(trimmed) > maxLoggedHTTPBodyBytes
	if truncated {
		trimmed = trimmed[:maxLoggedHTTPBodyBytes]
	}

	return summarizeWebhookBody(trimmed), truncated
}

func enableOutgoingHTTPLogging(logger zerolog.Logger) {
	next := http.DefaultTransport
	if next == nil {
		// Should not be nil in go, but just in case
		next = new(http.Transport)
	}
	http.DefaultClient.Transport = &loggingRoundTripper{
		next:   next,
		logger: logger,
	}
}
