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

package alertmanager_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leinardi/gotilert/internal/alertmanager"
)

var errConnectionRefused = errors.New("connection refused")

func TestShouldRetryContextCanceledFalse(t *testing.T) {
	t.Parallel()

	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, context.Canceled)

	if alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=false for context.Canceled")
	}
}

func TestShouldRetryContextDeadlineExceededFalse(t *testing.T) {
	t.Parallel()

	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, context.DeadlineExceeded)

	if alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=false for context.DeadlineExceeded")
	}
}

func TestShouldRetryTimeoutTrue(t *testing.T) {
	t.Parallel()

	timeoutErr := &net.DNSError{IsTimeout: true, Err: "timeout"}
	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, timeoutErr)

	if !alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=true for timeout net.Error")
	}
}

func TestShouldRetryOpErrorTrue(t *testing.T) {
	t.Parallel()

	operationErr := &net.OpError{Op: "dial", Err: errConnectionRefused}
	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, operationErr)

	if !alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=true for net.OpError")
	}
}

func TestShouldRetryX509UnknownAuthorityFalse(t *testing.T) {
	t.Parallel()

	x509Err := x509.UnknownAuthorityError{}
	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, x509Err)

	if alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=false for x509.UnknownAuthorityError")
	}
}

func TestShouldRetryTLSRecordHeaderFalse(t *testing.T) {
	t.Parallel()

	recordErr := tls.RecordHeaderError{Msg: "http: server gave HTTP response to HTTPS client"}
	requestErr := fmt.Errorf("%w: %w", alertmanager.ErrDoRequest, recordErr)

	if alertmanager.ShouldRetry(requestErr) {
		t.Fatalf("expected ShouldRetry=false for tls.RecordHeaderError")
	}
}

func TestPostAlertsRetriesOn500AndEventuallySucceeds(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	upstream := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/v2/alerts" {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			current := requestCount.Add(1)

			// Fail twice, then succeed.
			if current <= 2 {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = writer.Write([]byte("boom\n"))

				return
			}

			writer.WriteHeader(http.StatusOK)
		}),
	)
	defer upstream.Close()

	client, err := alertmanager.New(&alertmanager.Options{
		BaseURL: upstream.URL,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("alertmanager.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	postErr := client.PostAlerts(ctx, []alertmanager.Alert{
		{
			Labels:   map[string]string{"alertname": "Test"},
			StartsAt: time.Now().UTC(),
			EndsAt:   time.Now().UTC().Add(1 * time.Minute),
		},
	})
	if postErr != nil {
		t.Fatalf("PostAlerts: expected success, got %v", postErr)
	}

	if gotCount := requestCount.Load(); gotCount != 3 {
		t.Fatalf("expected 3 attempts, got %d", gotCount)
	}
}

func TestPostAlertsDoesNotRetryOn400(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	upstream := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/api/v2/alerts" {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			requestCount.Add(1)

			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte("bad request\n"))
		}),
	)
	defer upstream.Close()

	client, err := alertmanager.New(&alertmanager.Options{
		BaseURL: upstream.URL,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("alertmanager.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	postErr := client.PostAlerts(ctx, []alertmanager.Alert{
		{
			Labels:   map[string]string{"alertname": "Test"},
			StartsAt: time.Now().UTC(),
			EndsAt:   time.Now().UTC().Add(1 * time.Minute),
		},
	})
	if postErr == nil {
		t.Fatalf("expected error, got nil")
	}

	if !errors.Is(postErr, alertmanager.ErrUpstreamNon2xx) {
		t.Fatalf("expected ErrUpstreamNon2xx, got %v", postErr)
	}

	if gotCount := requestCount.Load(); gotCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", gotCount)
	}
}
