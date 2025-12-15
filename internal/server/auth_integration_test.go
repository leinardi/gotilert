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

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leinardi/gotilert/internal/gotify"
	"github.com/leinardi/gotilert/internal/server"
)

func TestAuthTokenPrecedenceHeaderWins(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]server.App{
		"HEADER": {Name: "app", ID: 1},
		"QUERY":  {Name: "app", ID: 1},
		"BEARER": {Name: "app", ID: 1},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message?token=QUERY",
		bytes.NewReader(mustJSON(t, gotify.MessageRequest{Message: "hello"})),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", "HEADER")
	req.Header.Set("Authorization", "Bearer BEARER")

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestAuthTokenPrecedenceQuerySecond(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]server.App{
		"QUERY":  {Name: "app", ID: 1},
		"BEARER": {Name: "app", ID: 1},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message?token=QUERY",
		bytes.NewReader(mustJSON(t, gotify.MessageRequest{Message: "hello"})),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer BEARER")

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestAuthTokenPrecedenceBearerLast(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]server.App{
		"BEARER": {Name: "app", ID: 1},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message",
		bytes.NewReader(mustJSON(t, gotify.MessageRequest{Message: "hello"})),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer BEARER")

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestAuthUnknownTokenForbidden(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, map[string]server.App{
		"KNOWN": {Name: "app", ID: 1},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"http://example.local/message",
		bytes.NewReader(mustJSON(t, gotify.MessageRequest{Message: "hello"})),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gotify-Key", "WRONG")

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf(
			"expected status %d, got %d body=%s",
			http.StatusForbidden,
			rec.Code,
			rec.Body.String(),
		)
	}
}

func newTestServer(t *testing.T, tokenToApp map[string]server.App) *http.Server {
	t.Helper()

	resolve := func(token string) (server.App, bool) {
		app, ok := tokenToApp[token]

		return app, ok
	}

	forward := func(_ context.Context, _ server.App, _ gotify.MessageRequest, _ uint64) error {
		return nil
	}

	httpServer, err := server.New(&server.Options{
		Addr:            "127.0.0.1:0",
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		IdleTimeout:     1 * time.Second,
		ShutdownTimeout: 1 * time.Second,
		MaxBodyBytes:    1 << 20,

		Health: func() (bool, string) { return true, "" },
		Ready:  func() (bool, string) { return true, "" },

		ResolveApp:     resolve,
		ForwardMessage: forward,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return httpServer
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	return data
}
