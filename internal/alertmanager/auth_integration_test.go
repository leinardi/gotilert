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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leinardi/gotilert/internal/alertmanager"
)

func TestClientBasicAuthAppliedToReadyAndPostAlerts(t *testing.T) {
	t.Parallel()

	const (
		wantUser = "user"
		wantPass = "pass"
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
			// Both endpoints must be protected.
			switch r.URL.Path {
			case "/-/ready", "/api/v2/alerts":
				user, pass, ok := r.BasicAuth()
				if !ok || user != wantUser || pass != wantPass {
					writer.Header().Set("WWW-Authenticate", `Basic realm="alertmanager"`)
					writer.WriteHeader(http.StatusUnauthorized)

					return
				}

				writer.WriteHeader(http.StatusOK)

				return
			default:
				writer.WriteHeader(http.StatusNotFound)

				return
			}
		}),
	)
	defer server.Close()

	client, err := alertmanager.New(&alertmanager.Options{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
		Auth: alertmanager.Auth{
			// Intentionally padded with whitespace to ensure trimming works.
			BasicUsername: "  " + wantUser + "  ",
			BasicPassword: "\n" + wantPass + "\t",
		},
	})
	if err != nil {
		t.Fatalf("alertmanager.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.Ready(ctx)
	if err != nil {
		t.Fatalf("Ready: expected no error, got %v", err)
	}

	err = client.PostAlerts(ctx, []alertmanager.Alert{
		{
			Labels:   map[string]string{"alertname": "Test"},
			StartsAt: time.Now().UTC(),
			EndsAt:   time.Now().UTC().Add(1 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("PostAlerts: expected no error, got %v", err)
	}
}

func TestClientUnauthorizedReturnsStatusErrorWithBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v2/alerts" {
				writer.WriteHeader(http.StatusNotFound)

				return
			}

			writer.Header().Set("WWW-Authenticate", `Basic realm="alertmanager"`)
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte("unauthorized\n"))
		}),
	)
	defer server.Close()

	client, err := alertmanager.New(&alertmanager.Options{
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
		Auth: alertmanager.Auth{
			BasicUsername: "wrong",
			BasicPassword: "wrong",
		},
	})
	if err != nil {
		t.Fatalf("alertmanager.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.PostAlerts(ctx, []alertmanager.Alert{
		{
			Labels:   map[string]string{"alertname": "Test"},
			StartsAt: time.Now().UTC(),
			EndsAt:   time.Now().UTC().Add(1 * time.Minute),
		},
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	if !errors.Is(err, alertmanager.ErrUpstreamNon2xx) {
		t.Fatalf("expected ErrUpstreamNon2xx, got %v", err)
	}

	var stErr alertmanager.HTTPStatusError
	if !errors.As(err, &stErr) {
		t.Fatalf("expected HTTPStatusError in chain, got %v", err)
	}

	if stErr.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, stErr.StatusCode())
	}

	if stErr.Body() != "unauthorized" {
		t.Fatalf("expected body %q, got %q", "unauthorized", stErr.Body())
	}
}
