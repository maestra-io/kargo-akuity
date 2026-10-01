package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/akuity/kargo/pkg/logging"
)

func TestStartMetricsServer(t *testing.T) {
	logger, err := logging.NewLogger(logging.InfoLevel, logging.ConsoleFormat)
	require.NoError(t, err)

	t.Run("disabled by default", func(t *testing.T) {
		addr := freeAddr(t)
		startMetricsServer(t.Context(), metricsDisabled, logger)

		_, getErr := http.Get(fmt.Sprintf("http://%s/metrics", addr)) // nolint: bodyclose
		require.Error(t, getErr)
	})

	t.Run("serves controller-runtime's registry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		addr := freeAddr(t)
		startMetricsServer(ctx, addr, logger)

		body := getEventually(t, fmt.Sprintf("http://%s/metrics", addr))
		// pkg/server registers its collectors when it starts serving, not at
		// package init -- otherwise every subcommand of this single binary
		// would export them (the controller Pods included). No server has been
		// started here, so they must be absent; pkg/server's tests cover the
		// registration itself.
		require.NotContains(t, body, "kargo_api_http_")
		// From controller-runtime's registry -- the reason the API server's
		// goroutine count becomes visible at all. The client-go
		// rest_client_* collectors live in the same registry but only emit
		// once the API server has actually called the Kubernetes API, so they
		// are not asserted here.
		require.Contains(t, body, "go_goroutines")
	})
}

// freeAddr returns a loopback address that was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func getEventually(t *testing.T, url string) string {
	t.Helper()
	var body string
	require.Eventually(
		t,
		func() bool {
			res, err := http.Get(url) // nolint: gosec
			if err != nil {
				return false
			}
			defer res.Body.Close()
			b, err := io.ReadAll(res.Body)
			if err != nil {
				return false
			}
			body = string(b)
			return res.StatusCode == http.StatusOK
		},
		5*time.Second,
		50*time.Millisecond,
	)
	return body
}
