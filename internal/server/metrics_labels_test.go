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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leinardi/gotilert/internal/metrics"
	"github.com/leinardi/gotilert/internal/server"
)

// Any client can send any path and any method token, so neither may become a label value as
// sent: each distinct value is a new series kept for the life of the process.
func TestRequestMetricsLabelBoundedRoutes(t *testing.T) {
	t.Parallel()

	metricsCollector := metrics.New()

	srv, err := server.New(&server.Options{Metrics: metricsCollector})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	for _, target := range []string{"/random-1", "/random-2/deeper", "/healthz"} {
		srv.Handler.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil),
		)
	}

	srv.Handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), "BREW", "/healthz", nil),
	)

	recorder := httptest.NewRecorder()
	metricsCollector.Handler().ServeHTTP(
		recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil),
	)

	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}

	exposition := string(body)

	for _, want := range []string{`path="unmatched"`, `path="/healthz"`, `method="OTHER"`} {
		if !strings.Contains(exposition, want) {
			t.Errorf("metrics lack %s:\n%s", want, exposition)
		}
	}

	for _, unwanted := range []string{"random-1", "random-2", `method="BREW"`} {
		if strings.Contains(exposition, unwanted) {
			t.Errorf("metrics carry the raw request value %s:\n%s", unwanted, exposition)
		}
	}
}
