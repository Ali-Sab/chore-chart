package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func handlerWithStatus(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	})
}

func TestNewMiddleware_Captures200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/schedule", nil)
	w := httptest.NewRecorder()
	NewMiddleware(handlerWithStatus(http.StatusOK)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNewMiddleware_Captures401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/secret", nil)
	w := httptest.NewRecorder()
	NewMiddleware(handlerWithStatus(http.StatusUnauthorized)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNewMiddleware_Captures404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	w := httptest.NewRecorder()
	NewMiddleware(handlerWithStatus(http.StatusNotFound)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestNewMiddleware_DefaultsTo200WhenWriteHeaderNotCalled(t *testing.T) {
	// Handler writes a body without calling WriteHeader — should default to 200.
	noExplicitHeader := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	NewMiddleware(noExplicitHeader).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 as default, got %d", w.Code)
	}
}

func TestResponseWriter_WriteHeaderOnce(t *testing.T) {
	// Calling WriteHeader twice should not panic and first code wins.
	doubleWrite := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusOK) // should be ignored by http.ResponseWriter
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, code: http.StatusOK}
	doubleWrite.ServeHTTP(rw, req)

	if rw.code != http.StatusCreated {
		t.Errorf("expected code 201 captured, got %d", rw.code)
	}
}

func TestNewMiddleware_PassesThroughBody(t *testing.T) {
	bodyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	req := httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	w := httptest.NewRecorder()
	NewMiddleware(bodyHandler).ServeHTTP(w, req)

	if body := w.Body.String(); body != `{"ok":true}` {
		t.Errorf("expected body passthrough, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}
