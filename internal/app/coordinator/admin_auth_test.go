package coordinator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdminAuth_NoAuthHeader(t *testing.T) {
	s := &Server{
		config: &Config{
			AdminAPIAuthToken: "test-admin-token-32-chars-long!!",
		},
	}

	handler := s.requireAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminAuth_InvalidToken(t *testing.T) {
	s := &Server{
		config: &Config{
			AdminAPIAuthToken: "test-admin-token-32-chars-long!!",
		},
	}

	handler := s.requireAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminAuth_ValidToken(t *testing.T) {
	adminToken := "test-admin-token-32-chars-long!!"
	s := &Server{
		config: &Config{
			AdminAPIAuthToken: adminToken,
		},
	}

	handlerCalled := false
	handler := s.requireAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !handlerCalled {
		t.Error("handler should have been called")
	}
}

func TestRequireAdminAuth_EmptyBearerToken(t *testing.T) {
	s := &Server{
		config: &Config{
			AdminAPIAuthToken: "test-admin-token-32-chars-long!!",
		},
	}

	handler := s.requireAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdminAuth_NonBearerAuth(t *testing.T) {
	s := &Server{
		config: &Config{
			AdminAPIAuthToken: "test-admin-token-32-chars-long!!",
		},
	}

	handler := s.requireAdminAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
