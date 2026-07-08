package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed static/index.html
var landingPageHTML []byte

// Config holds the application configuration
type Config struct {
	ProjectID   string
	SecretName  string
	Environment string
	Port        string
	Host        string
	SecretValue string
}

// URLShortener holds the in-memory database of short codes to long URLs
type URLShortener struct {
	mu    sync.RWMutex
	store map[string]string
}

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

var (
	// Prometheus metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "url_shortener_http_requests_total",
			Help: "Total number of HTTP requests processed, labeled by status, method, and path.",
		},
		[]string{"status", "method", "path"},
	)
	redirectsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "url_shortener_redirects_total",
			Help: "Total number of successful short URL redirects, labeled by code.",
		},
		[]string{"code"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "url_shortener_request_duration_seconds",
			Help:    "HTTP request latency histogram.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	// Register Prometheus metrics
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(redirectsTotal)
	prometheus.MustRegister(requestDuration)
}

func main() {
	// 1. Initialize Logger
	env := os.Getenv("ENVIRONMENT")
	var logger *slog.Logger
	if env == "production" || env == "staging" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	slog.SetDefault(logger)

	// 2. Load Configuration
	config := loadConfig(logger)

	// 3. Initialize URL Shortener store
	shortener := &URLShortener{
		store: make(map[string]string),
	}

	// 4. Setup Routes (Using Go 1.22 wildcard features in ServeMux)
	mux := http.NewServeMux()

	// App API endpoints
	mux.HandleFunc("POST /shorten", shortener.handleShorten)
	mux.HandleFunc("GET /{code}", shortener.handleRedirect)

	// Serve static landing page
	mux.HandleFunc("GET /{$}", handleLandingPage)

	// Liveness and Readiness Probes
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /health", handleHealthz)

	// Metrics endpoint
	mux.Handle("GET /metrics", promhttp.Handler())

	// Apply Middlewares (Logging, Secure Headers, Metrics)
	wrappedHandler := middlewareMetricsAndHeaders(mux)

	// 5. Start Server
	addr := fmt.Sprintf("%s:%s", config.Host, config.Port)
	slog.Info("Starting server", "addr", addr, "env", config.Environment)

	server := &http.Server{
		Addr:         addr,
		Handler:      wrappedHandler,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server startup failed", "error", err)
		os.Exit(1)
	}
}

// loadConfig reads configs and resolves secrets from GCP Secret Manager if configured
func loadConfig(logger *slog.Logger) Config {
	config := Config{
		ProjectID:   os.Getenv("GCP_PROJECT_ID"),
		SecretName:  os.Getenv("SECRET_NAME"),
		Environment: os.Getenv("ENVIRONMENT"),
		Port:        os.Getenv("PORT"),
		Host:        os.Getenv("HOST"),
	}

	if config.Port == "" {
		config.Port = "8080"
	}
	if config.Host == "" {
		// Default to localhost for security, but allow 0.0.0.0 when containerized
		config.Host = "127.0.0.1"
		if os.Getenv("CONTAINERIZED") == "true" {
			config.Host = "0.0.0.0"
		}
	}
	if config.SecretName == "" {
		config.SecretName = "url-shortener-secret"
	}

	// Try loading secret from GCP Secret Manager
	if os.Getenv("SECRET_MANAGER_TYPE") == "gcp" && config.ProjectID != "" {
		logger.Info("Attempting to fetch secret from GCP Secret Manager", "project_id", config.ProjectID, "secret_name", config.SecretName)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			logger.Warn("Failed to initialize GCP Secret Manager client, falling back to local env variables", "error", err)
		} else {
			defer client.Close()
			secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", config.ProjectID, config.SecretName)
			req := &secretmanagerpb.AccessSecretVersionRequest{Name: secretPath}
			result, err := client.AccessSecretVersion(ctx, req)
			if err != nil {
				logger.Warn("Failed to access Secret Manager secret, falling back to local env variables", "error", err, "path", secretPath)
			} else {
				config.SecretValue = string(result.Payload.Data)
				logger.Info("Successfully retrieved secret from GCP Secret Manager")
			}
		}
	}

	// Fallback to local environment variable if SecretValue is still empty
	if config.SecretValue == "" {
		config.SecretValue = os.Getenv("APP_SECRET_VALUE")
		if config.SecretValue == "" {
			logger.Warn("No APP_SECRET_VALUE provided. Running with warning.")
		} else {
			logger.Info("Loaded secret from local environment variable")
		}
	}

	return config
}

// Middleware: Applies security headers, logs requests, and records Prometheus metrics
func middlewareMetricsAndHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Secure Headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none';")

		// Create a custom response writer to capture status code
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		// Record duration metric
		duration := time.Since(start).Seconds()
		requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)

		// Record total requests metric
		statusStr := fmt.Sprintf("%d", sw.status)
		httpRequestsTotal.WithLabelValues(statusStr, r.Method, r.URL.Path).Inc()

		// Log request details
		slog.Info("Request handled",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", duration*1000,
			"user_agent", r.UserAgent(),
		)
	})
}

// statusWriter wraps http.ResponseWriter to track status
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// handleHealthz implements the liveness and readiness probe
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// handleShorten accepts a long URL, stores it, and returns the short URL response
func (u *URLShortener) handleShorten(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode shorten request body", "error", err)
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate input URL to mitigate SSRF and redirection vulnerabilities
	parsedURL, err := url.ParseRequestURI(req.URL)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		slog.Warn("Attempted to shorten invalid URL", "url", req.URL)
		http.Error(w, `{"error": "Invalid URL scheme. Only HTTP and HTTPS are permitted."}`, http.StatusBadRequest)
		return
	}

	// Generate a short code
	code, err := generateRandomCode(6)
	if err != nil {
		slog.Error("Failed to generate short code", "error", err)
		http.Error(w, `{"error": "Internal server error"}`, http.StatusInternalServerError)
		return
	}

	u.mu.Lock()
	u.store[code] = req.URL
	u.mu.Unlock()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	shortURL := fmt.Sprintf("%s://%s/%s", scheme, r.Host, code)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ShortenResponse{
		Code:     code,
		ShortURL: shortURL,
	})
}

// handleRedirect redirects the client to the original long URL mapped to the short code
func (u *URLShortener) handleRedirect(w http.ResponseWriter, r *http.Request) {
	// In Go 1.22, wildcards are retrieved using PathValue
	code := r.PathValue("code")

	u.mu.RLock()
	longURL, exists := u.store[code]
	u.mu.RUnlock()

	if !exists {
		slog.Warn("Short code lookup failed", "code", code)
		http.Error(w, `{"error": "Short URL not found"}`, http.StatusNotFound)
		return
	}

	// Increment redirect metric
	redirectsTotal.WithLabelValues(code).Inc()

	// Redirect to the original URL (using 302 Found)
	// TODO(security): The redirect URL is validated on input (/shorten), but we should ensure safety
	http.Redirect(w, r, longURL, http.StatusFound)
}

// generateRandomCode builds a cryptographically secure random alphanumeric string
func generateRandomCode(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b), nil
}

// handleLandingPage serves the embedded index.html file
func handleLandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(landingPageHTML)
}
