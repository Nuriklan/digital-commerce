package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry

	// HTTP Metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Business metrics
	OrdersCreatedTotal  prometheus.Counter
	PaymentsFailedTotal *prometheus.CounterVec
}

// New initializes and registers all the metrics in isolated register.
func New(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()

	// Adding standard Go process and execution time metrics (memory, goroutines, GC)
	reg.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
	)

	m := &Metrics{
		Registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "digital_commerce",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests processed, partitioned by method, path pattern, and status code.",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"method", "path", "status"},
		),

		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "digital_commerce",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "Latency of HTTP requests in seconds.",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
				// Standard delay intervals: from 5 ms to 10 seconds
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),

		OrdersCreatedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "digital_commerce",
				Subsystem: "order",
				Name:      "created_total",
				Help:      "Total number of orders successfully created.",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
		),

		PaymentsFailedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "digital_commerce",
				Subsystem: "payment",
				Name:      "failed_total",
				Help:      "Total number of failed payments by reason.",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"reason"},
		),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.OrdersCreatedTotal,
		m.PaymentsFailedTotal,
	)

	return m
}

// Handler returns an HTTP handler for scraping Prometheus metrics (/metrics).
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
