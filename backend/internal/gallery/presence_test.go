package gallery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPresenceCountsBrowsersAndExpiresIdleVisitors(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "presence.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tracker := newPresenceTracker(store)
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	router := gin.New()
	router.POST("/api/presence", tracker.heartbeat)
	heartbeat := func(cookie *http.Cookie) (int, *http.Cookie) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
		request.Header.Set("X-Gallery-Analytics-Consent", "accepted")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("presence response: %d %s", response.Code, response.Body.String())
		}
		var result struct {
			Online int `json:"online"`
		}
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
	if count, cookie := heartbeat(first); count != 1 || cookie != nil {
		t.Fatal("idle visits must expire while retaining the browser identity")
	}
	usage, err := store.Usage(t.Context(), now, 7)
	if err != nil || usage.Visitors != 2 || usage.Visits != 3 {
		t.Fatalf("returning browser should start a new visit: %+v %v", usage, err)
	}
}

func TestPresencePersistsAndStopsDurationAtTheLastHeartbeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "presence.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	beat := func(cookie *http.Cookie) *http.Cookie {
		t.Helper()
		tracker := newPresenceTracker(store)
		tracker.now = func() time.Time { return now }
		router := gin.New()
		router.POST("/api/presence", tracker.heartbeat)
		req := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
		req.Header.Set("X-Gallery-Analytics-Consent", "accepted")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		if cookies := response.Result().Cookies(); len(cookies) > 0 {
			return cookies[0]
		}
		return cookie
	}
	cookie := beat(nil)
	now = now.Add(20 * time.Second)
	beat(cookie)
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(20 * time.Second)
	beat(cookie)
	now = now.Add(60 * time.Second)
	usage, err := store.Usage(t.Context(), now, 7)
	if err != nil || usage.Online != 0 || usage.Visits != 1 || usage.TotalSeconds != 40 {
		t.Fatalf("history did not survive restart or idle was counted: %+v %v", usage, err)
	}
	beat(cookie)
	usage, err = store.Usage(t.Context(), now, 7)
	if err != nil || usage.Visitors != 1 || usage.Visits != 2 || usage.TotalSeconds != 40 {
		t.Fatalf("idle time counted: %+v %v", usage, err)
	}
}

func TestCountryRequiresConfiguredProxyHeader(t *testing.T) {
	for _, check := range []struct{ header, value, want string }{
		{"", "TH", ""}, {"CF-IPCountry", "th", "TH"}, {"CF-IPCountry", "XX", ""}, {"CF-IPCountry", "T1", ""}, {"CF-IPCountry", "Thailand", ""}, {"CF-IPCountry", "<>", ""}, {"CF-IPCountry", "QQ", ""}, {"CF-IPCountry", "EU", ""},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
		request.Header.Set("CF-IPCountry", check.value)
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		if got := presenceCountry(context, check.header); got != check.want {
			t.Errorf("%q %q: got %q, want %q", check.header, check.value, got, check.want)
		}
	}
}

func TestConcurrentTabsShareOneVisitAndCountry(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "presence.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("GALLERY_COUNTRY_HEADER", "CF-IPCountry")
	router := Router(store, "test-presence-administrator-24chars")
	var visitors sync.WaitGroup
	for range 8 {
		visitors.Go(func() {
			request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
			request.Header.Set("X-Gallery-Analytics-Consent", "accepted")
			request.AddCookie(&http.Cookie{Name: "gallery_visitor", Value: "0123456789abcdef0123456789abcdef"})
			request.Header.Set("CF-IPCountry", "TH")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Errorf("concurrent heartbeat failed: %d %s", response.Code, response.Body.String())
			}
		})
	}
	visitors.Wait()
	usage, err := store.Usage(t.Context(), time.Now(), 7)
	if err != nil || usage.Online != 1 || usage.Visitors != 1 || usage.Visits != 1 || len(usage.Countries) != 1 || usage.Countries[0].Country != "TH" {
		t.Fatalf("tabs were counted separately or country was lost: %+v %v", usage, err)
	}
}

func TestPresenceDoesNotTrackWithoutConsentAndWithdrawalClearsTheCookie(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "consent.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := Router(store, "test-presence-administrator-24chars")
	for _, consent := range []string{"", "rejected", "true"} {
		request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
		request.Header.Set("X-Gallery-Analytics-Consent", consent)
		request.AddCookie(&http.Cookie{Name: "gallery_visitor", Value: "0123456789abcdef0123456789abcdef"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unconsented heartbeat: %d %s", response.Code, response.Body.String())
		}
	}
	usage, err := store.Usage(t.Context(), time.Now(), 7)
	if err != nil || usage.Visits != 0 || usage.Online != 0 {
		t.Fatalf("unconsented browser was tracked: %+v %v", usage, err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/presence", nil)
	request.Header.Set("X-Gallery-Analytics-Consent", "accepted")
	accepted := httptest.NewRecorder()
	router.ServeHTTP(accepted, request)
	if accepted.Code != http.StatusOK {
		t.Fatalf("accepted analytics failed: %d", accepted.Code)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/presence", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusNoContent || len(cookies) != 1 || cookies[0].Name != "gallery_visitor" || cookies[0].MaxAge != -1 || cookies[0].Path != "/api/presence" {
		t.Fatalf("withdrawal did not clear the visitor cookie: %d %v", response.Code, cookies)
	}
	usage, err = store.Usage(t.Context(), time.Now(), 7)
	if err != nil || usage.Visits != 1 {
		t.Fatalf("withdrawal changed prior consented history: %+v %v", usage, err)
	}
}
