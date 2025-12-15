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

package alertmanager

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout      = 5 * time.Second
	maxErrorBodyBytes       = 64 * 1024
	defaultRetryMaxAttempts = 3
	defaultRetryInitial     = 200 * time.Millisecond
	defaultRetryMaxBackoff  = 1 * time.Second
)

var ErrContextDone = errors.New("context done")

type Auth struct {
	BasicUsername string
	BasicPassword string
	BearerToken   string
}

type Options struct {
	BaseURL            string
	Timeout            time.Duration
	InsecureSkipVerify bool
	Auth               Auth
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	auth       Auth

	retryMaxAttempts int
	retryInitial     time.Duration
	retryMaxBackoff  time.Duration
}

// HTTPStatusError is returned (wrapped) when Alertmanager responds with a non-2xx status.
// It exposes the HTTP status code and a limited response body excerpt for debugging.
type HTTPStatusError interface {
	error
	StatusCode() int
	Body() string
}

type statusError struct {
	statusCode int
	body       string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("alertmanager returned non-2xx status: %d", e.statusCode)
}

func (e *statusError) StatusCode() int {
	return e.statusCode
}

func (e *statusError) Body() string {
	return e.body
}

func New(opts *Options) (*Client, error) {
	if opts == nil {
		return nil, ErrInvalidConfiguration
	}

	baseURLRaw := strings.TrimSpace(opts.BaseURL)
	if baseURLRaw == "" {
		return nil, ErrBaseURLMissing
	}

	parsed, err := url.Parse(baseURLRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfiguration, err)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultHTTPTimeout
	}

	tlsConfig := &tls.Config{} //nolint:gosec // user-configured option; explicitly supported for self-signed homelab setups.
	tlsConfig.InsecureSkipVerify = opts.InsecureSkipVerify

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("%w: default transport has unexpected type", ErrInvalidConfiguration)
	}

	transport := baseTransport.Clone()
	transport.TLSClientConfig = tlsConfig

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	return &Client{
		baseURL:    parsed,
		httpClient: httpClient,
		auth:       normalizeAuth(opts.Auth),

		retryMaxAttempts: defaultRetryMaxAttempts,
		retryInitial:     defaultRetryInitial,
		retryMaxBackoff:  defaultRetryMaxBackoff,
	}, nil
}

func normalizeAuth(auth Auth) Auth {
	auth.BasicUsername = strings.TrimSpace(auth.BasicUsername)
	auth.BasicPassword = strings.TrimSpace(auth.BasicPassword)
	auth.BearerToken = strings.TrimSpace(auth.BearerToken)

	return auth
}

func (client *Client) PostAlerts(ctx context.Context, alerts []Alert) error {
	if client == nil || client.httpClient == nil || client.baseURL == nil {
		return ErrClientNil
	}

	attempts := max(client.retryMaxAttempts, 1)

	for attempt := 1; attempt <= attempts; attempt++ {
		err := client.postAlertsOnce(ctx, alerts)
		if err == nil {
			return nil
		}

		// If context is already canceled/deadline exceeded, stop immediately.
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return fmt.Errorf("%w: %w", ErrDoRequest, ctxErr)
		}

		// Decide whether retry is appropriate.
		if !shouldRetry(err) || attempt == attempts {
			return err
		}

		backoff := computeBackoff(attempt, client.retryInitial, client.retryMaxBackoff)

		sleepErr := sleepWithContext(ctx, backoff)
		if sleepErr != nil {
			return fmt.Errorf("%w: %w", ErrDoRequest, sleepErr)
		}
	}

	return ErrDoRequest
}

func (client *Client) applyAuth(req *http.Request) {
	if req == nil {
		return
	}

	// Prefer bearer when present.
	if client.auth.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+client.auth.BearerToken)

		return
	}

	// Only apply BasicAuth when any creds are provided (config validation should ensure both).
	if client.auth.BasicUsername != "" || client.auth.BasicPassword != "" {
		req.SetBasicAuth(client.auth.BasicUsername, client.auth.BasicPassword)
	}
}

func (client *Client) postAlertsOnce(ctx context.Context, alerts []Alert) error {
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: "/api/v2/alerts"})

	bodyBytes, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrEncodeRequest, err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreateRequest, err)
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client.applyAuth(req)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDoRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		limitedReader := io.LimitReader(resp.Body, maxErrorBodyBytes)

		data, readErr := io.ReadAll(limitedReader)
		if readErr != nil {
			return fmt.Errorf("%w: %w", ErrReadResponseBody, readErr)
		}

		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}

		statusErr := &statusError{
			statusCode: resp.StatusCode,
			body:       msg,
		}

		// Keep a stable sentinel for callers/linting, but preserve status/body for retry decisions.
		return fmt.Errorf("%w: %w", ErrUpstreamNon2xx, statusErr)
	}

	return nil
}

// ShouldRetry reports whether an Alertmanager operation should be retried for the given error.
// It is exported so it can be tested from the external test package (alertmanager_test).
func ShouldRetry(err error) bool {
	return shouldRetry(err)
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Never retry caller-driven cancellations/deadlines.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Permanent TLS/cert issues won't improve with retries.
	if isPermanentTLSError(err) {
		return false
	}

	// Retry on upstream status codes: 429 + 5xx.
	var statusErr *statusError
	if errors.As(err, &statusErr) {
		code := statusErr.StatusCode()
		if code == http.StatusTooManyRequests {
			return true
		}

		return code >= http.StatusInternalServerError
	}

	// Retry on explicit transport execution wrapper (network-ish failures).
	if errors.Is(err, ErrDoRequest) {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return true
		}

		// Many connection failures come as *net.OpError (e.g. connection refused).
		// Bounded retries are still helpful for brief hiccups.
		var opErr *net.OpError

		return errors.As(err, &opErr)
	}

	// Fallback: don't retry unknown error types.
	return false
}

func isPermanentTLSError(err error) bool {
	// x509 verification failures are permanent unless config/certs change.
	var unknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthorityErr) {
		return true
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}

	var certificateInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certificateInvalidErr) {
		return true
	}

	var systemRootsErr x509.SystemRootsError
	if errors.As(err, &systemRootsErr) {
		return true
	}

	// Common when HTTPS is configured against an HTTP endpoint (or similar mismatch).
	var recordHeaderErr tls.RecordHeaderError

	return errors.As(err, &recordHeaderErr)
}

func computeBackoff(attempt int, initial, maxBackoff time.Duration) time.Duration {
	if attempt <= 1 {
		return initial
	}

	backoff := initial
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= maxBackoff {
			return maxBackoff
		}
	}

	if backoff > maxBackoff {
		return maxBackoff
	}

	return backoff
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrContextDone, ctx.Err())
	case <-timer.C:
		return nil
	}
}
