package landing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mpepping/discovery-service/internal/state"
	"go.uber.org/zap"
)

func TestServeHealth(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	handler, err := NewHandler(st, logger)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := w.Body.String()
	if body != `{"status":"healthy"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestServeReady(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	handler, err := NewHandler(st, logger)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := w.Body.String()
	if body != `{"status":"ready"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestServeReadyNotInitialized(t *testing.T) {
	logger := zap.NewNop()
	handler := &Handler{
		state:  nil, // Not initialized
		logger: logger,
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestServeNotFound(t *testing.T) {
	logger := zap.NewNop()
	st := state.NewState(logger)
	handler, err := NewHandler(st, logger)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}
