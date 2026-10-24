package gallery

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Opt-in only: this calls the public finder and, when configured, bills one TypeSafe request.
func TestExampleFinderLiveReportedBOPayload(t *testing.T) {
	origin := os.Getenv("GALLERY_FINDER_LIVE_URL")
	if origin == "" {
		t.Skip("set GALLERY_FINDER_LIVE_URL to run against a real finder")
	}
	t.Setenv("GANK_FINDER_URL", origin)
	store, err := Open(t.TempDir() + "/live-finder.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := Router(store, "live-test-administrator-token-24chars")
	req := httptest.NewRequest("POST", "/api/admin/example-candidates", strings.NewReader(`{"seller":"girlskpopphoto","setDate":"2026-10-24","name":"261018 sullyoon","group":"NMIXX","count":0}`))
	req.Header.Set("Authorization", "Bearer live-test-administrator-token-24chars")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("lookup returned %d: %s", w.Code, w.Body.String())
	}
	var result exampleSuggestions
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) == 0 {
		t.Fatal("reported BO payload returned no candidates")
	}
	for _, candidate := range result.Candidates {
		t.Logf("candidate: %s (%s)", candidate.URL, candidate.Title)
	}
	t.Logf("recommended=%s warning=%s", result.RecommendedURL, result.Warning)
	if result.Confidence != nil {
		t.Logf("confidence=%.2f", *result.Confidence)
	}
}
