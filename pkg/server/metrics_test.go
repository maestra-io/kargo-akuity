package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestClassifyRoute(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "REST API is measured by the Gin middleware",
			path:     "/v1beta1/projects/kargo-demo/stages/test",
			expected: routeSkip,
		},
		{
			name:     "ConnectRPC procedure keeps its own label",
			path:     connectRoutePrefix + "ListStages",
			expected: connectRoutePrefix + "ListStages",
		},
		{
			name:     "gRPC health checks are collapsed",
			path:     healthRoutePrefix + "Check",
			expected: routeHealth,
		},
		{
			name:     "Dex proxy is collapsed",
			path:     "/dex/.well-known/openid-configuration",
			expected: routeDex,
		},
		{
			name:     "UI bundle is collapsed",
			path:     "/assets/index-abc123.js",
			expected: routeUI,
		},
		{
			name:     "UI index is collapsed",
			path:     "/",
			expected: routeUI,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.expected, classifyRoute(testCase.path))
		})
	}
}

func TestInstrumentHandler(t *testing.T) {
	testCases := []struct {
		name          string
		path          string
		code          int
		expectedRoute string
	}{
		{
			name:          "successful ConnectRPC call",
			path:          connectRoutePrefix + "ListStages",
			code:          http.StatusOK,
			expectedRoute: connectRoutePrefix + "ListStages",
		},
		{
			name: "unknown ConnectRPC procedure is collapsed, so an " +
				"arbitrary caller cannot mint label values",
			path:          connectRoutePrefix + "NotAProcedure",
			code:          http.StatusNotFound,
			expectedRoute: routeConnectUnknown,
		},
		{
			name:          "unauthenticated ConnectRPC call keeps its route",
			path:          connectRoutePrefix + "ListProjects",
			code:          http.StatusUnauthorized,
			expectedRoute: connectRoutePrefix + "ListProjects",
		},
		{
			name:          "UI asset that does not exist stays collapsed",
			path:          "/assets/nope.js",
			code:          http.StatusNotFound,
			expectedRoute: routeUI,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			httpRequestsTotal.Reset()

			handler := instrumentHandler(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(testCase.code)
				}),
			)
			handler.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodPost, testCase.path, nil),
			)

			require.Equal(
				t,
				float64(1),
				testutil.ToFloat64(
					httpRequestsTotal.WithLabelValues(
						testCase.expectedRoute,
						http.MethodPost,
						strconv.Itoa(testCase.code),
					),
				),
			)
			require.Equal(t, 1, testutil.CollectAndCount(httpRequestsTotal))
		})
	}
}

func TestInstrumentHandlerSkipsREST(t *testing.T) {
	httpRequestsTotal.Reset()

	handler := instrumentHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/v1beta1/projects", nil),
	)

	// The Gin middleware owns REST requests; counting them here as well would
	// double every REST series.
	require.Equal(t, 0, testutil.CollectAndCount(httpRequestsTotal))
}

func TestInstrumentHandlerPreservesFlusher(t *testing.T) {
	var flushable bool
	handler := instrumentHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Streaming procedures (ConnectRPC Watch*) depend on this.
			_, flushable = w.(http.Flusher)
			w.WriteHeader(http.StatusOK)
		}),
	)
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, connectRoutePrefix+"WatchStages", nil),
	)
	require.True(t, flushable)
}

func TestGinMetricsMiddleware(t *testing.T) {
	testCases := []struct {
		name          string
		register      string
		request       string
		expectedRoute string
		expectedCode  int
	}{
		{
			name: "matched route is labeled by template, not by the " +
				"concrete project name",
			register:      "/v1beta1/projects/:project/stages/:stage",
			request:       "/v1beta1/projects/kargo-demo/stages/test",
			expectedRoute: "/v1beta1/projects/:project/stages/:stage",
			expectedCode:  http.StatusOK,
		},
		{
			name:          "unmatched route is collapsed",
			register:      "/v1beta1/projects",
			request:       "/v1beta1/nope",
			expectedRoute: routeRESTUnmatched,
			expectedCode:  http.StatusNotFound,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			httpRequestsTotal.Reset()

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(ginMetricsMiddleware())
			router.GET(testCase.register, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})
			router.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, testCase.request, nil),
			)

			require.Equal(t, 1, testutil.CollectAndCount(httpRequestsTotal))
			require.Equal(
				t,
				float64(1),
				testutil.ToFloat64(
					httpRequestsTotal.WithLabelValues(
						testCase.expectedRoute,
						http.MethodGet,
						strconv.Itoa(testCase.expectedCode),
					),
				),
			)
		})
	}
}

func TestRegisterMetrics(t *testing.T) {
	// Idempotent: Serve calls it, and a second call must not panic with
	// duplicate registration.
	registerMetrics()
	registerMetrics()

	families, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)

	var names []string
	for _, family := range families {
		if strings.HasPrefix(family.GetName(), "kargo_api_") {
			names = append(names, family.GetName())
		}
	}
	// Only the gauge is asserted: a *Vec with no children emits no family at
	// all, and which requests other tests in this package have recorded is not
	// this test's business.
	require.Contains(t, names, "kargo_api_http_requests_in_flight")
}
