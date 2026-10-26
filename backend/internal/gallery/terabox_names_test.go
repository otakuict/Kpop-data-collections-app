package gallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTeraBoxNamesUsesSavedAccountRootAndDoesNotCreatePreviews(t *testing.T) {
	store := teraTestStore(t)
	key := "names-test-key"
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, id := range []int{1, 2} {
		root := "/First"
		if id == 2 {
			root = "/Second"
		}
		if err := store.saveTeraAccount(context.Background(), key, teraAccount{ID: id, Label: root, Root: root}, teraSession{Cookie: "ndus=" + root}); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("dir") != "/Second" || r.Header.Get("Cookie") != "ndus=/Second" {
			t.Errorf("wrong account or root: %s", r.URL)
		}
		json.NewEncoder(w).Encode(teraPage{Errno: new(int), List: []teraFile{
			{ID: 1, Name: "recent.jpg", Path: "/Second/recent.jpg", Modified: now.Add(-time.Hour).Unix(), Created: now.Unix()},
			{ID: 2, Name: "240404 Artist", Path: "/Second/240404 Artist", IsDir: 1, Created: now.Add(-20 * 24 * time.Hour).Unix()},
			{ID: 3, Name: "no-time.txt", Path: "/Second/no-time.txt"},
		}})
	}))
	defer server.Close()
	service := teraService{store: store, key: key, now: func() time.Time { return now }, provider: teraClient{baseURL: server.URL, client: server.Client()}}
	recent, err := service.names(context.Background(), 2, 5)
	if err != nil || len(recent.Entries) != 1 || recent.Entries[0].Name != "recent.jpg" || recent.Root != "/Second" || !recent.Complete {
		t.Fatalf("result=%+v error=%v", recent, err)
	}
	if recent.Entries[0].ModifiedAt != "2026-10-26T11:00:00Z" {
		t.Fatalf("modification time=%q", recent.Entries[0].ModifiedAt)
	}
	all, err := service.names(context.Background(), 2, 0)
	if err != nil || len(all.Entries) != 3 || calls != 2 {
		t.Fatalf("result=%+v calls=%d error=%v", all, calls, err)
	}
	if all.Entries[1].ModifiedAt != "" || all.Entries[2].ModifiedAt != "" {
		t.Fatal("missing modification time must remain empty, including creation-only entries")
	}
	for _, table := range []string{"terabox_previews", "terabox_imports"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d error=%v", table, count, err)
		}
	}
	for _, days := range []int{-1, 11} {
		if _, err := service.names(context.Background(), 2, days); err == nil {
			t.Fatal("invalid days accepted")
		}
	}
	if _, err := service.names(context.Background(), 3, 5); err == nil {
		t.Fatal("invalid account accepted")
	}
}

func TestTeraBoxNamesRequiresAuthenticationAndValidSettings(t *testing.T) {
	store := teraTestStore(t)
	key := "names-test-administrator-key-24chars"
	router := Router(store, key)
	for _, body := range []string{`{"accountId":0}`, `{"accountId":1,"days":11}`, `{"accountId":1,"days":-1}`, `{`} {
		req := httptest.NewRequest("POST", "/api/admin/terabox/names", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("body=%s status=%d", body, response.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/admin/terabox/names", strings.NewReader(`{"accountId":1}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 401 {
		t.Fatalf("unauthenticated status=%d", response.Code)
	}
}

func TestTeraBoxNamesListsOnlyRootPreservingProviderOrderAcrossPagesAndLookback(t *testing.T) {
	store := teraTestStore(t)
	key := "names-order-test-key"
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	if err := store.saveTeraAccount(context.Background(), key, teraAccount{ID: 1, Label: "Drive", Root: "/"}, teraSession{Cookie: "ndus=test"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dir") != "/" {
			t.Errorf("names must not request a subfolder: %s", r.URL.Query().Get("dir"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("order") != "time" || r.URL.Query().Get("desc") != "1" {
			t.Error("provider ordering changed")
		}
		more := r.URL.Query().Get("page") == "1"
		files := []teraFile{{ID: 5, Name: "b.jpg", Path: "/b.jpg", Modified: now.Unix()}}
		if more {
			files = []teraFile{
				{ID: 30, Name: "z.zip", Path: "/z.zip", Modified: now.Add(-2 * time.Hour).Unix()},
				{ID: 10, Name: "a.jpg", Path: "/a.jpg", Modified: now.Add(-20 * 24 * time.Hour).Unix()},
				{ID: 20, Name: "Limited 260814 Ahyeon", Path: "/Limited 260814 Ahyeon", IsDir: 1, Modified: now.Add(-time.Hour).Unix()},
			}
		}
		json.NewEncoder(w).Encode(teraPage{Errno: new(int), HasMore: &more, List: files})
	}))
	defer server.Close()
	service := teraService{store: store, key: key, now: func() time.Time { return now }, provider: teraClient{baseURL: server.URL, client: server.Client()}}
	for _, check := range []struct {
		days int
		want []int64
	}{{0, []int64{30, 10, 20, 5}}, {5, []int64{30, 20, 5}}} {
		result, err := service.names(context.Background(), 1, check.days)
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, entry := range result.Entries {
			ids = append(ids, entry.FileID)
		}
		if !reflect.DeepEqual(ids, check.want) {
			t.Fatalf("days=%d order=%v want=%v", check.days, ids, check.want)
		}
	}
}
