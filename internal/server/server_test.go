package server

import (
	"helloapp/internal/metrics"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleLivez(t *testing.T) {
	m := metrics.New()
	srv := New(":0", m, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	srv.handleLivez(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "ok\n" {
		t.Errorf("expected 'ok\\n', got %q", body)
	}
}
func TestHandleHealthz_NoRedis(t *testing.T) {
	m := metrics.New()
	srv := New(":0", m, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.handleHealthz(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
func TestHandleRoot(t *testing.T) {
	m := metrics.New()
	srv := New(":0", m, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.handleRoot(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "Hello!\n" {
		t.Errorf("expected 'Hello!\\n', got %q", body)
	}
}
func TestHandleBoom(t *testing.T) {
	m := metrics.New()
	srv := New(":0", m, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	srv.handleBoom(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
func TestRequestIDMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromCtx(r.Context())
		if rid == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := withRequestID(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id response header")
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Request-Id", "test-id-123")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if got := w2.Header().Get("X-Request-Id"); got != "test-id-123" {
		t.Errorf("expected X-Request-Id=test-id-123, got %q", got)
	}
}
