package gallery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminSessionExchangeAndExpiry(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "session.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secret := "test-operator-token-at-least-24-chars"
	router := Router(store, secret)
	call := func(method, path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if got := call(http.MethodPost, "/api/admin/session", "wrong"); got.Code != 401 {
		t.Fatalf("invalid administrator token accepted: %d", got.Code)
	}
	response := call(http.MethodPost, "/api/admin/session", secret)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("session exchange failed: %d %s", response.Code, response.Body.String())
	}
	var session struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || session.Token == "" || session.ExpiresIn != 86400 {
		t.Fatalf("invalid session response: %s %v", response.Body.String(), err)
	}
	if got := call(http.MethodGet, "/api/admin/status", session.Token); got.Code != 200 {
		t.Fatalf("JWT rejected: %d", got.Code)
	}
	if got := call(http.MethodGet, "/api/admin/ingestion", session.Token); got.Code != 200 {
		t.Fatalf("JWT rejected by admin route: %d", got.Code)
	}
	if got := call(http.MethodGet, "/api/admin/status", secret); got.Code != 200 {
		t.Fatalf("legacy bearer token rejected: %d", got.Code)
	}
	parts := strings.Split(session.Token, ".")
	parts[1] = strings.Repeat("A", len(parts[1]))
	if got := call(http.MethodGet, "/api/admin/status", strings.Join(parts, ".")); got.Code != 401 {
		t.Fatalf("tampered JWT accepted: %d", got.Code)
	}
	if got := call(http.MethodGet, "/api/admin/status", issueAdminJWT(secret, time.Now().Add(-adminSessionLifetime-time.Minute))); got.Code != 401 {
		t.Fatalf("expired JWT accepted: %d", got.Code)
	}
	if validAdminJWT(session.Token, "another-operator-token-at-least-24-chars", time.Now()) {
		t.Fatal("old JWT survived administrator token rotation")
	}
}
