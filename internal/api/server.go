package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	apiRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ollama_api_requests_total",
			Help: "Total number of HTTP requests to the Ollama API server",
		},
		[]string{"method", "path", "status"},
	)

	apiRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ollama_api_request_duration_seconds",
			Help:    "Duration of HTTP requests to the Ollama API server",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// Config holds the configuration for the API server
type Config struct {
	BindAddress string
	APIKey      string
	Namespace   string
}

// Server represents the HTTP API server
type Server struct {
	config       Config
	client       client.Client
	router       *mux.Router
	server       *http.Server
	shutdownChan chan struct{}
}

// NewServer creates a new API server instance
func NewServer(config Config, k8sClient client.Client) *Server {
	router := mux.NewRouter()
	server := &Server{
		config:       config,
		client:       k8sClient,
		router:       router,
		shutdownChan: make(chan struct{}),
	}

	// Setup routes
	router.Use(server.metricsMiddleware)
	router.Use(server.authMiddleware)

	// API v1 routes
	apiV1 := router.PathPrefix("/api/v1").Subrouter()

	// Models endpoints
	apiV1.HandleFunc("/models", server.listModels).Methods(http.MethodGet)
	apiV1.HandleFunc("/models", server.createModel).Methods(http.MethodPost)
	apiV1.HandleFunc("/models/{name}", server.getModel).Methods(http.MethodGet)
	apiV1.HandleFunc("/models/{name}", server.deleteModel).Methods(http.MethodDelete)
	apiV1.HandleFunc("/models/{name}/refresh", server.refreshModel).Methods(http.MethodPost)

	// Health check endpoints
	router.HandleFunc("/health", server.healthCheck).Methods(http.MethodGet)
	router.HandleFunc("/readiness", server.readinessCheck).Methods(http.MethodGet)

	return server
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("api-server")
	logger.Info("starting API server", "address", s.config.BindAddress)

	s.server = &http.Server{
		Addr:         s.config.BindAddress,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "API server failed to start")
			close(s.shutdownChan)
		}
	}()

	return nil
}

// Shutdown stops the API server
func (s *Server) Shutdown(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("api-server")
	logger.Info("shutting down API server")

	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// NeedLeaderElection implements the LeaderElectionRunnable interface.
// The API server doesn't need leader election.
func (s *Server) NeedLeaderElection() bool {
	return false
}

// metricsMiddleware is a middleware that collects metrics about API requests
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer that captures the status code
		rw := &responseWriter{w, http.StatusOK}

		// Call the next handler
		next.ServeHTTP(rw, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		apiRequestsTotal.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", rw.statusCode)).Inc()
		apiRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

// authMiddleware handles authentication for the API
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check endpoints
		if r.URL.Path == "/health" || r.URL.Path == "/readiness" {
			next.ServeHTTP(w, r)
			return
		}

		// Check the API key if configured
		if s.config.APIKey != "" {
			apiKey := r.Header.Get("X-API-Key")
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(s.config.APIKey)) != 1 {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// healthCheck handles the health check endpoint
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// readinessCheck handles the readiness check endpoint
func (s *Server) readinessCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and passes it to the wrapped ResponseWriter
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// sendJSON helper function to send JSON responses
func sendJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// sendError helper function to send error responses
func sendError(w http.ResponseWriter, err error, status int) {
	errorRes := map[string]string{"error": err.Error()}
	sendJSON(w, errorRes, status)
}
