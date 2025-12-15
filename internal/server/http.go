/*
 * MIT License
 *
 * Copyright (c) 2025 Roberto Leinardi
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

// Package server exposes the minimal HTTP surface of Gotilert.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/leinardi/gotilert/internal/logger"
	"github.com/leinardi/gotilert/internal/metrics"
)

const (
	metricsPath = "/metrics"
	healthzPath = "/healthz"
	readyzPath  = "/readyz"
	messagePath = "/message"

	okBody = "ok\n"
)

var ErrServerNil = errors.New("http server is nil")

// HealthFunc returns whether the service is healthy and, if not, a short reason.
type HealthFunc func() (bool, string)

// ReadyFunc returns whether the service is ready and, if not, a short reason.
type ReadyFunc func() (bool, string)

type Options struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	MaxBodyBytes int64

	Health HealthFunc
	Ready  ReadyFunc

	ResolveApp     ResolveAppFunc
	ForwardMessage ForwardMessageFunc

	Metrics *metrics.Metrics
}

// New returns a configured *http.Server with handlers and timeouts.
func New(opts *Options) (*http.Server, error) {
	if opts == nil {
		return nil, ErrServerOptionsNil
	}

	mux := http.NewServeMux()

	healthFunc := opts.Health
	if healthFunc == nil {
		healthFunc = func() (bool, string) { return true, "" }
	}

	readyFunc := opts.Ready
	if readyFunc == nil {
		readyFunc = func() (bool, string) { return true, "" }
	}

	maxBodyBytes := opts.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = 1 << 20 // 1 MiB
	}

	mux.HandleFunc(healthzPath, healthHandler(healthFunc))
	mux.HandleFunc(readyzPath, readyHandler(readyFunc))
	mux.HandleFunc(messagePath, messageHandler(opts.ResolveApp, opts.ForwardMessage, maxBodyBytes))

	if opts.Metrics != nil {
		mux.Handle(metricsPath, opts.Metrics.Handler())
	}

	handler := withRequestLogging(opts.Metrics, mux)

	srv := &http.Server{
		Addr:         opts.Addr,
		Handler:      handler,
		ReadTimeout:  opts.ReadTimeout,
		WriteTimeout: opts.WriteTimeout,
		IdleTimeout:  opts.IdleTimeout,
	}

	return srv, nil
}

// ListenAndServe starts the server and blocks until it exits.
// It returns http.ErrServerClosed on normal shutdown.
func ListenAndServe(srv *http.Server) error {
	if srv == nil {
		return ErrServerNil
	}

	err := srv.ListenAndServe()
	if err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server with the given timeout.
func Shutdown(ctx context.Context, srv *http.Server, timeout time.Duration) error {
	if srv == nil {
		return ErrServerNil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := srv.Shutdown(shutdownCtx)
	if err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	return nil
}

func healthHandler(isHealthy HealthFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, _ *http.Request) {
		writePlainText(responseWriter)

		ok, reason := isHealthy()
		if ok {
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(responseWriter, okBody)

			return
		}

		responseWriter.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(responseWriter, normalizeReason(reason))
	}
}

func readyHandler(isReady ReadyFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, _ *http.Request) {
		writePlainText(responseWriter)

		ok, reason := isReady()
		if ok {
			responseWriter.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(responseWriter, okBody)

			return
		}

		responseWriter.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(responseWriter, normalizeReason(reason))
	}
}

func writePlainText(responseWriter http.ResponseWriter) {
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
}

func normalizeReason(reason string) string {
	trimmed := reason
	if trimmed == "" {
		trimmed = "unhealthy"
	}

	return trimmed + "\n"
}

type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (recorder *statusRecorder) WriteHeader(code int) {
	recorder.status = code
	recorder.ResponseWriter.WriteHeader(code)
}

func withRequestLogging(metricsCollector *metrics.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		start := time.Now()

		recorder := &statusRecorder{
			ResponseWriter: responseWriter,
			status:         http.StatusOK,
		}

		next.ServeHTTP(recorder, request)

		duration := time.Since(start)

		logger.L().Info("http request",
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration", duration.String(),
		)

		if metricsCollector != nil {
			// Path cardinality is low (fixed endpoints).
			metricsCollector.ObserveRequest(
				request.Method,
				request.URL.Path,
				recorder.status,
				duration,
			)
		}
	})
}
