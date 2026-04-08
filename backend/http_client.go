package main

import (
"net/http"
"time"

"github.com/rs/zerolog"
)

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
	}
	
	event.Msg("outgoing http request")

	return resp, err
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
