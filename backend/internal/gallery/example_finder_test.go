package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExampleFinderSearchAndRecommendation(t *testing.T) {
	var logs bytes.Buffer
	previousLog := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousLog)

	var finderCalls, aiCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			finderCalls++
			q := r.URL.Query()
			if r.Method != "GET" || q.Get("seller") != "studio" || q.Get("date") != "261024" || q.Get("name") != "Sana" || q.Get("count") != "200" || q.Get("source") != "both" || q.Get("mode") != "broad" || q.Get("includeInactive") != "true" {
				t.Errorf("finder query: %v", q)
			}
			if r.Header.Get("Authorization") != "" {
				t.Error("TypeSafe key leaked to finder")
			}
			w.Write([]byte(`{"results":[{"title":"Sana A","postUrl":"https://ganknow.com/post/a","shopUrl":"https://ganknow.com/services/1-a","excerpt":"261024 Sana 200p","rank":1},{"title":"duplicate","postUrl":"https://ganknow.com/post/a"},{"title":"invalid","postUrl":"javascript:alert(1)"},{"title":"other host","postUrl":"https://evil.test/a"}]}`))
		case "/v1/systemone":
			aiCalls++
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("missing API key")
			}
			var body struct {
				Model     string       `json:"model"`
				State     exampleQuery `json:"state"`
				Questions map[string]struct {
					Type     string         `json:"type"`
					Criteria map[string]any `json:"criteria"`
				} `json:"questions"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "jev-latest" || body.State.Name != "Sana" || body.State.SetDate != "2026-10-24" || body.Questions["best"].Type != "choice" || len(body.Questions["best"].Criteria) != 3 {
				t.Errorf("invalid TypeSafe request: %+v", body)
			}
			w.Write([]byte(`{"answers":{"best":{"type":"choice","choice":"candidate_1","confidence":0.8}}}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer upstream.Close()
	finder := exampleFinder{baseURL: upstream.URL, apiKey: "test-key", model: "jev-latest", aiURL: upstream.URL + "/v1/systemone", client: upstream.Client()}
	result, err := finder.search(context.Background(), exampleQuery{Seller: "studio", SetDate: "2026-10-01", Name: "261024 Sana", Count: 200})
	if err != nil {
		t.Fatal(err)
	}

	for _, stage := range []string{"find_links", "filter_links", "recommend_link", "find_links_total"} {
		if !strings.Contains(logs.String(), "stage="+stage+" ") {
			t.Fatalf("missing timing stage %s: %s", stage, logs.String())
		}
	}
	if finderCalls != 1 || aiCalls != 1 || len(result.Candidates) != 2 || result.RecommendedURL != "https://ganknow.com/services/1-a" || result.Confidence == nil || *result.Confidence != 0.8 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExampleFinderAIErrorKeepsCandidates(t *testing.T) {
	for _, reply := range []string{`{"answers":{"best":{"type":"choice","choice":"invented","confidence":1}}}`, `{"answers":{"best":{"type":"choice","choice":"candidate_0","confidence":2}}}`, `{}`, `not json`} {
		t.Run(reply, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.Write([]byte(`{"results":[{"postUrl":"https://ganknow.com/post/a"}]}`))
					return
				}
				w.Write([]byte(reply))
			}))
			defer upstream.Close()
			finder := exampleFinder{baseURL: upstream.URL, apiKey: "test-key", model: "jev-latest", aiURL: upstream.URL, client: upstream.Client()}
			result, err := finder.search(context.Background(), exampleQuery{Seller: "studio", Name: "Sana"})
			if err != nil || len(result.Candidates) != 1 || result.RecommendedURL != "" || result.Warning == "" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestExampleQueryRejectsInvalidFilters(t *testing.T) {
	for _, q := range []exampleQuery{{Seller: "https://evil.test", Name: "Sana"}, {Seller: "studio"}, {Seller: "studio", Name: "Sana", SetDate: "261024"}, {Seller: "studio", Name: strings.Repeat("a", 101)}, {Seller: "studio", Name: "Sana", Count: -1}, {Seller: "studio", Name: "Sana", Count: 10000000}} {
		if _, err := q.parameters(); err == nil {
			t.Errorf("accepted %+v", q)
		}
	}
}

func TestExampleFinderHTTPRequiresAdminAndDoesNotSave(t *testing.T) {
	store, err := Open(t.TempDir() + "/finder.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{"results":[]}`)) }))
	defer upstream.Close()
	t.Setenv("GANK_FINDER_URL", upstream.URL)
	router := Router(store, "test-administrator-token-24chars")
	call := func(body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/admin/example-candidates", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := call(`{"seller":"studio","name":"Sana"}`, ""); w.Code != 401 {
		t.Fatalf("unauthorized status %d", w.Code)
	}
	if calls != 0 {
		t.Fatal("unauthorized caller reached finder")
	}
	if w := call(`{"seller":"studio"}`, "test-administrator-token-24chars"); w.Code != 400 {
		t.Fatalf("invalid query status %d", w.Code)
	}
	w := call(`{"seller":"studio","name":"Sana"}`, "test-administrator-token-24chars")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"candidates":[]`) || calls != 1 {
		t.Fatalf("%d %s calls=%d", w.Code, w.Body.String(), calls)
	}
	page, err := store.List(context.Background(), Filter{Page: 1, Limit: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("lookup saved metadata: %+v %v", page, err)
	}
}

func TestExampleFinderEmptyMissingKeyAndUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name, reply            string
		status                 int
		wantError, wantWarning bool
	}{
		{"empty", `{"results":[]}`, 200, false, false},
		{"missing key", `{"results":[{"postUrl":"https://ganknow.com/post/a"}]}`, 200, false, true},
		{"upstream failure", `{}`, 503, true, false},
		{"invalid JSON", `{`, 200, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("unexpected AI call")
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.reply))
			}))
			defer upstream.Close()
			f := exampleFinder{baseURL: upstream.URL, client: upstream.Client()}
			result, err := f.search(context.Background(), exampleQuery{Seller: "studio", Name: "Sana"})
			if (err != nil) != tc.wantError || (result.Warning != "") != tc.wantWarning {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestExampleGankURL(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "http://ganknow.com/post/a", "https://evil.test/post/a", "https://ganknow.com.evil.test/post/a", "https://user:pass@ganknow.com/post/a", "https://ganknow.com:443/post/a", "https://ganknow.com/profile/a"} {
		if _, ok := exampleGankURL(raw); ok {
			t.Errorf("accepted %s", raw)
		}
	}
	if got, ok := exampleGankURL("https://www.ganknow.com/post/a?tracking=1#preview"); !ok || got != "https://ganknow.com/post/a" {
		t.Fatalf("normalized %s %v", got, ok)
	}
}

func TestExampleFinderLimitsCandidatesAndAcceptsNoRecommendation(t *testing.T) {
	records := make([]map[string]string, 65)
	for i := range records {
		records[i] = map[string]string{"postUrl": fmt.Sprintf("https://ganknow.com/post/%d", i)}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"results": records})
			return
		}
		w.Write([]byte(`{"answers":{"best":{"type":"choice","choice":"none","confidence":0.7}}}`))
	}))
	defer upstream.Close()
	f := exampleFinder{baseURL: upstream.URL, apiKey: "test-key", model: "jev-latest", aiURL: upstream.URL, client: upstream.Client()}
	result, err := f.search(context.Background(), exampleQuery{Seller: "studio", Name: "Sana"})
	if err != nil || len(result.Candidates) != 60 || !result.Truncated || result.RecommendedURL != "" || result.Warning == "" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestExampleFinderConfigurationAndRedirects(t *testing.T) {
	t.Setenv("GANK_FINDER_URL", "")
	t.Setenv("TYPESAFE_API_KEY", "test-key")
	f := exampleFinderFromEnv()
	receivedKey := ""
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer redirect.Close()
	req, _ := http.NewRequest("POST", redirect.URL, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-key")
	var result any
	if err := f.decode(req, &result); err == nil {
		t.Fatal("redirect accepted")
	}
	if receivedKey != "" {
		t.Fatal("credential forwarded on redirect")
	}
	store, err := Open(t.TempDir() + "/finder.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := Router(store, "test-administrator-token-24chars")
	request := httptest.NewRequest("POST", "/api/admin/example-candidates", strings.NewReader(`{"seller":"studio","name":"Sana"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test-administrator-token-24chars")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != 503 {
		t.Fatalf("missing config: %d", w.Code)
	}
}

func TestExampleQueryUsesBroadSearchAndDateFromName(t *testing.T) {
	for _, tc := range []struct {
		name         string
		query        exampleQuery
		date, person string
	}{
		{"reported BO payload", exampleQuery{Seller: "girlskpopphoto", SetDate: "2026-10-24", Name: "261018 sullyoon"}, "261018", "sullyoon"},
		{"plain name", exampleQuery{Seller: "girlskpopphoto", SetDate: "2026-10-18", Name: "sullyoon"}, "261018", "sullyoon"},
		{"prefix without selected date", exampleQuery{Seller: "girlskpopphoto", Name: "261018 sullyoon"}, "261018", "sullyoon"},
		{"invalid prefix remains a name", exampleQuery{Seller: "studio", SetDate: "2026-10-24", Name: "261399 sullyoon"}, "261024", "261399 sullyoon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values, err := tc.query.parameters()
			if err != nil {
				t.Fatal(err)
			}
			if values.Get("date") != tc.date || values.Get("name") != tc.person || values.Get("mode") != "broad" || values.Get("source") != "both" || values.Get("includeInactive") != "true" {
				t.Fatalf("unexpected finder query: %v", values)
			}
		})
	}
}
