package gallery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAnalyticsCountsBrowsersVisitsAndObservedTime(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	start := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC).Unix()
	for _, visit := range []struct {
		visitor, country string
		start, end       int64
	}{
		{"one", "TH", start, start + 40},
		{"one", "TH", start + 200, start + 220},
		{"two", "", start, start},
	} {
		_, err := store.db.Exec(`INSERT INTO usage_visits(visitor_id,country,started_at,last_seen) VALUES(?,?,?,?)`, visit.visitor, visit.country, visit.start, visit.end)
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Usage(t.Context(), time.Unix(start+230, 0), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visitors != 2 || got.Visits != 3 || got.Online != 1 || got.AverageSeconds != 20 || got.TotalSeconds != 60 {
		t.Fatalf("unexpected totals: %+v", got)
	}
	if len(got.Daily) != 7 || got.Daily[6].Visitors != 2 || got.Daily[6].Visits != 3 || got.Daily[6].AverageSeconds != 20 {
		t.Fatalf("unexpected daily buckets: %+v", got.Daily)
	}
	if len(got.Countries) != 2 || got.Countries[0].Country != "TH" || got.Countries[0].Visitors != 1 || got.Countries[0].Visits != 2 {
		t.Fatalf("unexpected countries: %+v", got.Countries)
	}
}

func TestAnalyticsClipsVisitsToUTCDaysAndSelectedRange(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -6).Truncate(24 * time.Hour).Unix()
	for _, interval := range [][2]int64{{start - 20, start + 20}, {start - 100, start - 80}, {now.Truncate(24*time.Hour).Unix() - 20, now.Truncate(24*time.Hour).Unix() + 20}} {
		if _, err := store.db.Exec(`INSERT INTO usage_visits(visitor_id,country,started_at,last_seen) VALUES('one','TH',?,?)`, interval[0], interval[1]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Usage(t.Context(), now, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visitors != 1 || got.Visits != 2 || got.TotalSeconds != 60 || got.Daily[0].TotalSeconds != 20 || got.Daily[5].TotalSeconds != 20 || got.Daily[6].TotalSeconds != 20 {
		t.Fatalf("range clipping failed: %+v", got)
	}
}

func TestAnalyticsIsPrivateAndValidatesRange(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := "test-analytics-administrator-24chars"
	router := Router(store, token)
	for _, check := range []struct {
		query, token string
		status       int
	}{
		{"", "", 401}, {"", token, 200}, {"?days=7", token, 200}, {"?days=90", token, 200}, {"?days=0", token, 400}, {"?days=365", token, 400}, {"?days=bad", token, 400},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/admin/analytics"+check.query, nil)
		request.Header.Set("Authorization", "Bearer "+check.token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != check.status {
			t.Fatalf("%s: got %d, want %d: %s", check.query, response.Code, check.status, response.Body.String())
		}
		if check.status == 200 {
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("analytics must not be cached")
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["visitors"] != float64(0) || body["countries"] == nil || body["daily"] == nil {
				t.Fatalf("invalid empty dashboard: %s", response.Body.String())
			}
		}
	}
}

func TestAnalyticsPurgesVisitsOlderThanNinetyDays(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "retention.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	for _, days := range []int{91, 1} {
		seen := now.AddDate(0, 0, -days).Unix()
		if _, err := store.db.Exec(`INSERT INTO usage_visits(visitor_id,country,started_at,last_seen) VALUES('one','TH',?,?)`, seen, seen); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Usage(t.Context(), now, 90); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM usage_visits`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("expired history remains: %d %v", remaining, err)
	}
}
