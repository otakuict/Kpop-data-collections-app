package gallery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPresenceCountsBrowsersAndExpiresIdleVisitors(t *testing.T) {
	tracker := newPresenceTracker()
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	router := gin.New()
	router.POST("/api/presence", tracker.heartbeat)
	heartbeat := func(cookie *http.Cookie) (int, *http.Cookie) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("presence response: %d %s", response.Code, response.Body.String())
		}
		var result struct{ Online int `json:"online"` }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		cookies := response.Result().Cookies()
		if len(cookies) > 0 {
			return result.Online, cookies[0]
		}
		return result.Online, nil
	}
	firstCount, first := heartbeat(nil)
	if firstCount != 1 || first == nil || !first.HttpOnly || first.SameSite != http.SameSiteStrictMode {
		t.Fatal("first visitor was not counted with a private cookie")
	}
	if count, cookie := heartbeat(first); count != 1 || cookie != nil {
		t.Fatal("same browser was counted twice")
	}
	if count, _ := heartbeat(nil); count != 2 {
		t.Fatal("second browser was not counted")
	}
	now = now.Add(presenceTTL)
	if count, cookie := heartbeat(first); count != 1 || cookie == nil || cookie.Value == first.Value {
		t.Fatal("idle visitors did not expire")
	}
}
