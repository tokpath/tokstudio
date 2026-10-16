// Package observability records operational metadata only. It never records
// prompts, generated content, cookies, credentials, query strings or bodies.
package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	Registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	jobs     *prometheus.CounterVec
}

func New() *Metrics {
	r := prometheus.NewRegistry()
	m := &Metrics{Registry: r, requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tokenhub_http_requests_total", Help: "HTTP requests by registered route and outcome."}, []string{"method", "route", "status_class"}), duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "tokenhub_http_request_duration_seconds", Help: "HTTP request duration, including streamed response lifetime.", Buckets: []float64{.01, .05, .1, .5, 1, 5, 15, 60, 300}}, []string{"method", "route"}), jobs: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "tokenhub_worker_jobs_total", Help: "Durable worker job attempts by type and outcome."}, []string{"kind", "outcome"})}
	r.MustRegister(m.requests, m.duration, m.jobs, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return m
}
func (m *Metrics) Handler() gin.HandlerFunc {
	return gin.WrapH(promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
}
func (m *Metrics) HTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if _, e := uuid.Parse(requestID); e != nil {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		default:
			method = "OTHER"
		}
		ctx, span := otel.Tracer("tokenhub/http").Start(ctx, method+" "+route, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", method), attribute.String("http.route", route), attribute.String("request.id", requestID)))
		c.Request = c.Request.WithContext(ctx)
		defer span.End()
		c.Next()
		status := c.Writer.Status()
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= 500 {
			span.SetStatus(codes.Error, "server error")
		}
		if route != "/metrics" {
			m.requests.WithLabelValues(method, route, strconv.Itoa(status/100)+"xx").Inc()
			m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		}
	}
}
func (m *Metrics) Job(kind string, failed bool) {
	// Never promote a caller-supplied subject or customer ID into a metric label.
	switch kind {
	case "mail", "delivery", "price", "expiry", "renewal", "payments", "registration", "qualification", "usage", "risk", "invoice", "retention", "export", "procurement", "notification":
	default:
		kind = "other"
	}
	outcome := "completed"
	if failed {
		outcome = "retry_or_review"
	}
	m.jobs.WithLabelValues(kind, outcome).Inc()
}
