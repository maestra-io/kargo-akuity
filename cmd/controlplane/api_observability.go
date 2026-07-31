package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/akuity/kargo/pkg/logging"
)

const (
	// metricsDisabled is the value controller-runtime uses to mean "off"; the
	// API server honors the same convention so METRICS_BIND_ADDRESS behaves
	// identically across every component.
	metricsDisabled = "0"

	observabilityReadHeaderTimeout = time.Minute
	observabilityShutdownTimeout   = 5 * time.Second
)

// startMetricsServer serves Prometheus metrics on addr until ctx is canceled.
// Unlike the controller and management-controller, the API server runs no
// controller-runtime manager, so it has to stand this up itself.
//
// controller-runtime's registry is the one served: it holds the API server's
// own HTTP metrics (see pkg/server/metrics.go), the Go runtime and process
// collectors, and the client-go rest_client_* metrics for calls to the
// Kubernetes API. Adding prometheus.DefaultGatherer alongside it would double
// every Go and process collector and make /metrics answer 500.
func startMetricsServer(
	ctx context.Context,
	addr string,
	logger *logging.Logger,
) {
	if addr == "" || addr == metricsDisabled {
		logger.Info("metrics endpoint is disabled")
		return
	}

	mux := http.NewServeMux()
	mux.Handle(
		"/metrics",
		promhttp.HandlerFor(
			ctrlmetrics.Registry,
			promhttp.HandlerOpts{ErrorHandling: promhttp.HTTPErrorOnError},
		),
	)
	serveObservability(ctx, "metrics", addr, mux, logger)
}

// startPprofServer serves net/http/pprof on addr until ctx is canceled. Left
// disabled unless PPROF_BIND_ADDRESS is set. A goroutine dump is the only way
// to tell a starved API server (requests queued behind GC or a single
// GOMAXPROCS thread) from a deadlocked one.
func startPprofServer(
	ctx context.Context,
	addr string,
	logger *logging.Logger,
) {
	if addr == "" || addr == metricsDisabled {
		return
	}

	// Handlers are registered on our own mux rather than relying on
	// net/http/pprof's init(), which registers on http.DefaultServeMux.
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	serveObservability(ctx, "pprof", addr, mux, logger)
}

func serveObservability(
	ctx context.Context,
	name string,
	addr string,
	handler http.Handler,
	logger *logging.Logger,
) {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: observabilityReadHeaderTimeout,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			observabilityShutdownTimeout,
		)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "error shutting down server", "server", name)
		}
	}()

	go func() {
		logger.Info("serving", "server", name, "address", addr)
		// A failure here must not take the API server down: metrics and pprof
		// are observability, not function.
		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "error serving", "server", name, "address", addr)
		}
	}()
}
