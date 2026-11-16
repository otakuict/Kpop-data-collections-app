package gallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTeraBoxSetShareUsesJevThresholdAndManualConfirmation(t *testing.T) {
	store := teraTestStore(t)
	ctx := context.Background()
	key := "set-share-test-key"
	set, err := store.Save(ctx, 0, SetDraft{Title: "261028 Anna", Group: "MEOVV", Source: "studio"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.saveTeraAccount(ctx, key, teraAccount{ID: 1, Label: "Studio drive", Root: "/Albums", Source: "studio"}, teraSession{Cookie: "ndus=test"}); err != nil {
		t.Fatal(err)
	}
	confidence := 0.8
	shareCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/list":
			if r.URL.Query().Get("dir") != "/Albums" {
				t.Errorf("unexpected directory: %s", r.URL.Query().Get("dir"))
			}
			json.NewEncoder(w).Encode(teraPage{Errno: new(int), List: []teraFile{
				{ID: 10, Name: "261028 Anna", Path: "/Albums/261028 Anna", IsDir: 1, Created: time.Now().Unix()},
				{ID: 11, Name: "261027 Anna", Path: "/Albums/261027 Anna", IsDir: 1, Created: time.Now().Add(-time.Hour).Unix()},
			}})
		case "/ai":
			var payload struct {
				Questions map[string]struct {
					Criteria map[string]any `json:"criteria"`
				} `json:"questions"`
			}
			if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Questions["best"].Criteria) != 3 {
				t.Error("Jev did not receive the two folder choices")
			}
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"best": map[string]any{"type": "choice", "choice": "folder_0", "confidence": confidence}}})
		case "/share/pset":
			shareCount++
			if err := r.ParseForm(); err != nil || r.Form.Get("period") != "7" || !strings.Contains(r.Form.Get("path_list"), "Anna") {
				t.Errorf("wrong share request: %v, %v", r.Form, err)
			}
			w.Write([]byte(`{"errno":0,"link":"https://1024terabox.com/s/1abc"}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	service := teraService{store: store, key: key, provider: teraClient{baseURL: server.URL, client: server.Client()}, ai: exampleFinder{apiKey: "test", model: "jev-latest", aiURL: server.URL + "/ai", client: server.Client()}, now: time.Now}
	shared, err := service.prepareSetShare(ctx, set.ID)
	if err != nil || shared.Status != "shared" || shared.Folder == nil || shared.Folder.FileID != 10 || shareCount != 1 {
		t.Fatalf("high confidence: %+v, shares=%d, err=%v", shared, shareCount, err)
	}
	confidence = 0.59
	review, err := service.prepareSetShare(ctx, set.ID)
	if err != nil || review.Status != "review" || review.Confidence == nil || *review.Confidence != 0.59 || len(review.Candidates) != 2 || shareCount != 1 {
		t.Fatalf("low confidence: %+v, shares=%d, err=%v", review, shareCount, err)
	}
	manual, err := service.shareSetFolder(ctx, set.ID, teraFolderChoice{AccountID: 1, FileID: 11, Path: "/forged/path"})
	if err != nil || manual.Status != "shared" || manual.Folder.Path != "/Albums/261027 Anna" || shareCount != 2 {
		t.Fatalf("manual confirmation: %+v, shares=%d, err=%v", manual, shareCount, err)
	}
	if _, err := service.shareSetFolder(ctx, set.ID, teraFolderChoice{AccountID: 1, FileID: 999}); err == nil || shareCount != 2 {
		t.Fatal("unknown folder was shared")
	}
}

func TestTeraBoxSetShareJevOnlySelectsProvidedFolders(t *testing.T) {
	answer := "folder_0"
	confidence := 0.6
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"best": map[string]any{"type": "choice", "choice": answer, "confidence": confidence}}})
	}))
	defer server.Close()
	service := teraService{ai: exampleFinder{apiKey: "test", model: "jev-latest", aiURL: server.URL, client: server.Client()}}
	set := Set{SetDraft: SetDraft{Title: "261028 Anna", Group: "MEOVV"}}
	choices := []teraFolderChoice{{AccountID: 1, FileID: 10, Name: "261028 Anna", Path: "/Albums/261028 Anna"}}
	selected, score, warning := service.jevShareChoice(context.Background(), set, choices)
	if selected == nil || selected.FileID != 10 || score == nil || *score != 0.6 || warning != "" {
		t.Fatalf("valid choice: %+v %v %q", selected, score, warning)
	}
	answer = "folder_99"
	selected, _, warning = service.jevShareChoice(context.Background(), set, choices)
	if selected != nil || warning == "" {
		t.Fatal("Jev selected a folder outside the supplied choices")
	}
	answer = "none"
	selected, _, warning = service.jevShareChoice(context.Background(), set, choices)
	if selected != nil || warning == "" {
		t.Fatal("Jev's none choice did not require manual review")
	}
}

func TestTeraBoxSetShareRanksKoreanAndGivenNameFoldersForJev(t *testing.T) {
	romanized, err := romanizeKorean(context.Background(), []string{"250222 공유빈", "241202 원영"})
	if err != nil {
		t.Fatal(err)
	}
	members := []artistMember{
		{Group: "TripleS", Name: "Gong Yubin", Aliases: []string{"공유빈"}},
		{Group: "IVE", Name: "Jang Won-young", Aliases: []string{"장원영"}},
	}
	choices := []teraFolderChoice{
		{FileID: 1, Name: "250222 공유빈", Romanized: romanized[0], Path: "/Albums/250222 공유빈", Activity: 1},
		{FileID: 2, Name: "241202 원영", Romanized: romanized[1], Path: "/Albums/241202 원영", Activity: 1},
	}
	for i := 0; i < 40; i++ {
		choices = append(choices, teraFolderChoice{FileID: int64(i + 10), Name: "250222 unrelated " + strconv.Itoa(i), Path: "/Albums/unrelated", Activity: 100})
	}
	for _, tc := range []struct {
		set    Set
		wantID int64
	}{
		{Set{SetDraft: SetDraft{Title: "250222 Gong Yubin", Group: "TripleS"}}, 1},
		{Set{SetDraft: SetDraft{Title: "241202 Jang Wonyoung", Group: "IVE"}}, 2},
	} {
		ranked := append([]teraFolderChoice(nil), choices...)
		for i := range ranked {
			ranked[i].Score = shareMatchScore(tc.set, teraAccount{}, ranked[i], members)
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			if ranked[i].Score != ranked[j].Score {
				return ranked[i].Score > ranked[j].Score
			}
			return ranked[i].Activity > ranked[j].Activity
		})
		if ranked[0].FileID != tc.wantID || ranked[0].Score <= ranked[1].Score {
			t.Fatalf("set=%s first=%+v second=%+v", tc.set.Title, ranked[0], ranked[1])
		}
	}
	bare := teraFolderChoice{Name: "Wonyoung", Path: "/Albums/Wonyoung"}
	if score := shareMatchScore(Set{SetDraft: SetDraft{Title: "241202 Jang Wonyoung", Group: "IVE"}}, teraAccount{}, bare, members); score < 80 {
		t.Fatalf("given name without a date scored %d", score)
	}
}

func TestTeraBoxSetShareRoutesRegister(t *testing.T) {
	router := Router(&Store{}, "route-test-token-at-least-24-characters")
	for _, path := range []string{"/api/admin/sets/11/terabox/share", "/api/admin/sets/11/terabox/share-confirm"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("route %s returned %d, want admin authorization", path, response.Code)
		}
	}
}
