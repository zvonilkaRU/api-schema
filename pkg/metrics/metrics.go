// Package metrics provides Prometheus metrics via client_golang.
// Exposes request duration histograms, request counts by status, and in-flight gauge.
package metrics

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics collects Prometheus HTTP metrics.
type Metrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestsInFlight prometheus.Gauge
}

// New creates a new Metrics collector with standard HTTP metrics.
func New() *Metrics {
	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency histogram.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
		requestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Current number of HTTP requests being served.",
		}),
	}
	prometheus.MustRegister(m.requestsTotal)
	prometheus.MustRegister(m.requestDuration)
	prometheus.MustRegister(m.requestsInFlight)
	return m
}

// Middleware returns Echo middleware that collects HTTP metrics.
func (m *Metrics) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path // fallback for unmatched routes
			}
			method := c.Request().Method

			m.requestsInFlight.Inc()
			start := time.Now()

			err := next(c)

			status := c.Response().Status
			duration := time.Since(start).Seconds()
			m.requestsInFlight.Dec()

			statusStr := strconv.Itoa(status)
			m.requestsTotal.WithLabelValues(method, path, statusStr).Inc()
			m.requestDuration.WithLabelValues(method, path, statusStr).Observe(duration)

			return err
		}
	}
}

// Handler serves Prometheus metrics via promhttp.
func (m *Metrics) Handler(c echo.Context) error {
	promhttp.Handler().ServeHTTP(c.Response(), c.Request())
	return nil
}
