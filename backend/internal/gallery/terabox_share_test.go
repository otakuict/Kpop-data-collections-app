package gallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTeraBoxShareLatestFolderUsesSelectedSessionAndNewestFolder(t *testing.T) {
	store := teraTestStore(t)
	key := "share-latest-test-key"
	for _, account := range []teraAccount{{ID: 1, Label: "First", Root: "/First"}, {ID: 2, Label: "Second", Root: "/Second"}} {
		if err := store.saveTeraAccount(context.Background(), key, account, teraSession{Cookie: "ndus=" + account.Label, JSToken: "js-secret"}); err != nil {
			t.Fatal(err)
		}
	}
	var sharedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "ndus=Second" {
			t.Errorf("wrong session sent: %s", r.Header.Get("Cookie"))
		}
		switch r.URL.Path {
		case "/api/list":
			if r.URL.Query().Get("dir") != "/Second" {
				t.Errorf("wrong root: %s", r.URL.Query().Get("dir"))
			}
			json.NewEncoder(w).Encode(teraPage{Errno: new(int), List: []teraFile{
				{ID: 1, Name: "newest file.jpg", Path: "/Second/newest file.jpg", Modified: 300},
				{ID: 2, Name: "older folder", Path: "/Second/older folder", IsDir: 1, Modified: 100},
				{ID: 3, Name: "latest folder", Path: "/Second/latest folder", IsDir: 1, Created: 200},
			}})
		case "/share/pset":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("wrong share request: %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			sharedPath = r.Form.Get("path_list")
			if r.Form.Get("period") != "7" || r.Form.Get("schannel") != "0" {
				t.Errorf("unexpected share options: %v", url.Values(r.Form))
			}
			w.Write([]byte(`{"errno":0,"data":{"link":"https://terabox.com/s/1abc"}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	service := teraService{store: store, key: key, now: time.Now, provider: teraClient{baseURL: server.URL, client: server.Client()}}
	shared, err := service.shareLatestFolder(context.Background(), 2, 7)
	if err != nil || shared.Path != "/Second/latest folder" || shared.Period != 7 || shared.Link != "https://terabox.com/s/1abc" || sharedPath != `["/Second/latest folder"]` {
		t.Fatalf("result=%+v path=%q error=%v", shared, sharedPath, err)
	}
}

func TestTeraBoxShareAcceptsTopLevelLink(t *testing.T) {
	for _, period := range []int{0, 1} {
		t.Run(strconv.Itoa(period), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/share/pset" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil || r.Form.Get("period") != strconv.Itoa(period) {
					t.Errorf("wrong share period: %v, %v", r.Form, err)
				}
				w.Write([]byte(`{"errno":0,"link":"https://1024terabox.com/s/1abc"}`))
			}))
			defer server.Close()
			client := teraClient{baseURL: server.URL, client: server.Client()}
			link, err := client.shareFolder(context.Background(), teraSession{Cookie: "ndus=test"}, "/Albums/latest", period)
			if err != nil || link != "https://1024terabox.com/s/1abc" {
				t.Fatalf("link=%q error=%v", link, err)
			}
		})
	}
}

func TestTeraBoxShareLatestRouteRequiresAdminAndValidAccount(t *testing.T) {
	router := Router(teraTestStore(t), "share-route-test-key-at-least-24-characters")
	for _, tc := range []struct {
		body       string
		authorized bool
		status     int
	}{
		{`{"accountId":1}`, false, 401},
		{`{"accountId":0}`, true, 400},
		{`{"accountId":3}`, true, 400},
		{`{"accountId":1,"path":"/other"}`, true, 400},
		{`{"accountId":1,"period":30}`, true, 400},
		{`{"accountId":1}`, true, 502},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/terabox/share-latest", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		if tc.authorized {
			req.Header.Set("Authorization", "Bearer share-route-test-key-at-least-24-characters")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tc.status {
			t.Fatalf("body=%s status=%d want=%d", tc.body, response.Code, tc.status)
		}
	}
}
