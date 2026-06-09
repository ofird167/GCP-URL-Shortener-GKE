package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateRandomCode(t *testing.T) {
	length := 6
	code, err := generateRandomCode(length)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(code) != length {
		t.Errorf("expected code length %d, got %d", length, len(code))
	}

	// Verify all characters are within the expected alphanumeric charset
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, char := range code {
		if !strings.ContainsRune(charset, char) {
			t.Errorf("unexpected character in code: %c", char)
		}
	}
}

func TestURLShortener_HandleShorten(t *testing.T) {
	shortener := &URLShortener{
		store: make(map[string]string),
	}

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "Valid HTTP URL",
			body:           `{"url": "http://example.com/some/path"}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Valid HTTPS URL",
			body:           `{"url": "https://google.com"}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "Invalid URL scheme",
			body:           `{"url": "ftp://google.com"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid URL path (no host)",
			body:           `{"url": "relative-path"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON",
			body:           `{invalid-json}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/shorten", bytes.NewBufferString(tt.body))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(shortener.handleShorten)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedStatus == http.StatusCreated {
				var resp ShortenResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(resp.Code) != 6 {
					t.Errorf("expected short code length 6, got %d", len(resp.Code))
				}
				if !strings.Contains(resp.ShortURL, resp.Code) {
					t.Errorf("expected short URL to contain code %s, got %s", resp.Code, resp.ShortURL)
				}
			}
		})
	}
}

func TestURLShortener_HandleRedirect(t *testing.T) {
	shortener := &URLShortener{
		store: map[string]string{
			"test12": "https://example.com/target",
		},
	}

	// 1. Success case
	req, err := http.NewRequest("GET", "/test12", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Set path value matching Go 1.22 routing
	req.SetPathValue("code", "test12")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(shortener.handleRedirect)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302 Found, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if location != "https://example.com/target" {
		t.Errorf("expected redirect location https://example.com/target, got %s", location)
	}

	// 2. Not found case
	reqNotFound, err := http.NewRequest("GET", "/nonexistent", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	reqNotFound.SetPathValue("code", "nonexistent")

	rrNotFound := httptest.NewRecorder()
	handler.ServeHTTP(rrNotFound, reqNotFound)

	if rrNotFound.Code != http.StatusNotFound {
		t.Errorf("expected status 404 Not Found, got %d", rrNotFound.Code)
	}
}
