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

package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec

	forwardedAlertsTotal  *prometheus.CounterVec
	upstreamFailuresTotal *prometheus.CounterVec
}

func New() *Metrics {
	reg := prometheus.NewRegistry()

	metrics := &Metrics{
		registry: reg,

		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gotilert_http_requests_total",
				Help: "Total number of HTTP requests handled.",
			},
			[]string{"method", "path", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "gotilert_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		forwardedAlertsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gotilert_forwarded_alerts_total",
				Help: "Total number of alerts successfully forwarded to Alertmanager.",
			},
			[]string{"app"},
		),
		upstreamFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gotilert_upstream_failures_total",
				Help: "Total number of failures when calling upstream Alertmanager.",
			},
			[]string{"app"},
		),
	}

	// Keep registration explicit (no init()).
	reg.MustRegister(
		metrics.requestsTotal,
		metrics.requestDuration,
		metrics.forwardedAlertsTotal,
		metrics.upstreamFailuresTotal,
	)

	return metrics
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveRequest(method, path string, status int, duration time.Duration) {
	if m == nil {
		return
	}

	statusStr := strconv.Itoa(status)
	m.requestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.requestDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())
}

func (m *Metrics) IncForwarded(app string) {
	if m == nil {
		return
	}

	m.forwardedAlertsTotal.WithLabelValues(app).Inc()
}

func (m *Metrics) IncUpstreamFailure(app string) {
	if m == nil {
		return
	}

	m.upstreamFailuresTotal.WithLabelValues(app).Inc()
}
