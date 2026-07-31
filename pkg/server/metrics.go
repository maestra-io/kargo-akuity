package server

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/akuity/kargo/api/service/v1alpha1/svcv1alpha1connect"
)

const (
	metricsNamespace = "kargo"
	metricsSubsystem = "api"

	// connectRoutePrefix is the path prefix under which the (deprecated)
	// ConnectRPC handler is mounted. Paths beneath it are procedure names from
	// a generated, and therefore bounded, set.
	connectRoutePrefix = "/" + svcv1alpha1connect.KargoServiceName + "/"
	// restRoutePrefix is the path prefix of the REST API. Requests beneath it
	// are measured by the Gin middleware instead, which knows the matched
	// route template.
	restRoutePrefix = "/v1beta1/"
	// healthRoutePrefix is the path prefix of the gRPC health service.
	healthRoutePrefix = "/grpc.health.v1.Health/"
	// dexRoutePrefix is the path prefix of the reverse proxy to Dex.
	dexRoutePrefix = "/dex/"

	// routeSkip is returned by classifyRoute for requests that are measured
	// somewhere else and must not be counted twice.
	routeSkip = ""
	// routeHealth collapses every gRPC health check into one series.
	routeHealth = "health"
	// routeDex collapses every request proxied to Dex into one series. The
	// path beyond /dex/ is chosen by Dex, not by us.
	routeDex = "dex"
	// routeUI collapses every request for the dashboard bundle (index.html and
	// its hashed assets) into one series.
	routeUI = "ui"
	// routeConnectUnknown collapses requests for a ConnectRPC procedure that
	// does not exist. Keeping these out of the `route` label matters: the
	// generated handler answers an unknown procedure with a 404, so an
	// arbitrary caller could otherwise mint unbounded label values.
	routeConnectUnknown = "connect-unknown"
	// routeRESTUnmatched collapses REST requests that matched no route.
	routeRESTUnmatched = "rest-unmatched"
)

// Metrics for the API server's HTTP surface. The Kargo API server runs no
// controller-runtime manager, so nothing else measures it: without these, a
// degrading API or UI is invisible to Prometheus and only shows up as user
// reports.
//
// Collectors are package-level, per the client_golang convention, and go into
// controller-runtime's registry rather than the default one -- that is the
// registry every other Kargo component exposes, and it already carries the Go,
// process and client-go (rest_client_*) collectors, so serving it gives one
// endpoint with no duplicate collectors. cmd/controlplane/api.go serves it on
// METRICS_BIND_ADDRESS.
//
// They are constructed here but registered in registerMetrics, called from
// Serve -- NOT with promauto at package init. cmd/controlplane is a single
// binary, so an init-time registration lands in every subcommand: the
// controller and management-controller Pods would export
// kargo_api_http_requests_in_flight too (permanently 0, since they serve no
// HTTP through this package), which reads as a live API server to anything
// checking for the metric's presence.
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "http_requests_total",
			Help: "Total number of HTTP requests handled by the API server, " +
				"by route, method and response status code.",
		},
		[]string{"route", "method", "code"},
	)

	// Note the `code` label is deliberately absent here: keeping it off the
	// histogram avoids multiplying bucket series by every status code, and
	// latency questions ("is the API slow?") are asked per route, not per
	// code.
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "http_request_duration_seconds",
			Help: "Latency of HTTP requests handled by the API server, by " +
				"route and method. ConnectRPC Watch* procedures are " +
				"server-streaming, so their observations measure the lifetime " +
				"of the stream, not the latency of a call.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
			},
		},
		[]string{"route", "method"},
	)

	// In-flight requests is the saturation signal. The API server has no
	// concurrency limit of its own, so a rising floor here -- with latency
	// rising and CPU flat -- is what a starved server looks like from the
	// outside.
	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being handled by the API server.",
		},
	)
)

var registerMetricsOnce sync.Once

// registerMetrics adds this package's collectors to controller-runtime's
// registry. Safe to call more than once; only the first call registers.
func registerMetrics() {
	registerMetricsOnce.Do(func() {
		ctrlmetrics.Registry.MustRegister(
			httpRequestsTotal,
			httpRequestDuration,
			httpRequestsInFlight,
		)
	})
}

// classifyRoute maps a request path onto a bounded `route` label value. It
// returns routeSkip for requests that another middleware measures.
func classifyRoute(path string) string {
	switch {
	case strings.HasPrefix(path, restRoutePrefix):
		return routeSkip
	case strings.HasPrefix(path, healthRoutePrefix):
		return routeHealth
	case strings.HasPrefix(path, dexRoutePrefix):
		return routeDex
	case strings.HasPrefix(path, connectRoutePrefix):
		return path
	default:
		return routeUI
	}
}

// observeRequest records one completed request.
func observeRequest(
	route string,
	method string,
	code int,
	duration time.Duration,
) {
	httpRequestsTotal.WithLabelValues(
		route,
		method,
		strconv.Itoa(code),
	).Inc()
	httpRequestDuration.WithLabelValues(route, method).Observe(duration.Seconds())
}

// instrumentHandler wraps next with request count, latency and in-flight
// metrics. It wraps the server's outermost handler (inside the CORS handler,
// if any), so each HTTP/2 stream -- rather than each connection -- is measured
// as one request.
//
// It sits outside the basePath handling, so a request path still carries that
// prefix here; basePath is trimmed before classification so the `route` label
// does not depend on where the server is mounted.
func instrumentHandler(next http.Handler, basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := classifyRoute(strings.TrimPrefix(r.URL.Path, basePath))
		if route == routeSkip {
			next.ServeHTTP(w, r)
			return
		}

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		start := time.Now()
		rw := &responseRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)

		if rw.code == http.StatusNotFound &&
			strings.HasPrefix(route, connectRoutePrefix) {
			route = routeConnectUnknown
		}
		observeRequest(route, r.Method, rw.code, time.Since(start))
	})
}

// ginMetricsMiddleware measures REST API requests. It runs inside Gin so it
// can label by matched route template (e.g.
// /v1beta1/projects/:project/stages/:stage) rather than by the concrete path,
// which would put every project and stage name into the metric.
func ginMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		httpRequestsInFlight.Dec()
		route := c.FullPath()
		if route == "" {
			route = routeRESTUnmatched
		}
		observeRequest(
			route,
			c.Request.Method,
			c.Writer.Status(),
			time.Since(start),
		)
	}
}

// responseRecorder captures the status code written to a ResponseWriter.
type responseRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.code = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

// Flush and Unwrap keep streaming responses working: ConnectRPC's Watch*
// procedures and the gRPC health watch flush after every message, and
// wrapping a ResponseWriter without forwarding that would buffer a stream
// until it ended.
func (r *responseRecorder) Flush() {
	//nolint:errcheck // Nothing useful to do with a flush error here.
	http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
