package gallery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTeraBoxRecentFoldersUseCreationNotPhotoDate(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour).Unix()
	old := now.Add(-4 * 24 * time.Hour).Unix()
	entries := []teraFile{
		{ID: 1, Name: "240404 지연2", Path: "/240404 지연2", IsDir: 1, Created: recent},
		{ID: 2, Name: "261024 안나", Path: "/261024 안나", IsDir: 1, Created: old},
		{ID: 3, Name: "261024 수아", Path: "/261024 수아", IsDir: 1},
		{ID: 4, Name: "photo.jpg", Path: "/photo.jpg", Created: recent},
	}
	got, warnings := recentTeraFolders(entries, now)
	if len(got) != 1 || got[0].ID != 1 || len(warnings) != 1 {
		t.Fatalf("folders=%+v warnings=%v", got, warnings)
	}
	date, name, err := splitTeraTitle(got[0].Name)
	if err != nil || date != "2024-04-04" || name != "지연2" {
		t.Fatalf("%q %q %v", date, name, err)
	}
	if _, _, err := splitTeraTitle("2251101 하연"); err == nil {
		t.Fatal("accepted invalid date prefix")
	}
}

func TestTeraBoxCredentialsAreIsolatedEncryptedAndNeverReturned(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tera.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, id := range []int{1, 2} {
		err = s.saveTeraAccount(ctx, "long-test-admin-key", teraAccount{ID: id, Label: "account", Root: "/", Cookie: "ndus=secret"}, teraSession{Cookie: "ndus=secret", BDSToken: "bds"})
		if err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := s.teraAccounts(ctx)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("%+v %v", accounts, err)
	}
	b, _ := json.Marshal(accounts)
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "bds") {
		t.Fatal("credential leaked")
	}
	session, err := s.teraSession(ctx, "long-test-admin-key", 2)
	if err != nil || session.Cookie != "ndus=secret" {
		t.Fatalf("%+v %v", session, err)
	}
	if _, err = s.teraSession(ctx, "wrong-key", 1); err == nil {
		t.Fatal("wrong key decrypted session")
	}
	var stored string
	if err = s.db.QueryRow("SELECT ciphertext FROM terabox_accounts WHERE id=1").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "secret") {
		t.Fatal("plaintext credential stored")
	}
}

func TestTeraBoxClientSendsSessionOnlyToProviderAndPaginates(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/list" || r.Header.Get("Cookie") != "ndus=test" || r.URL.Query().Get("bdstoken") != "bds" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page") == "1" && r.URL.Query().Get("dir") == "/" {
			json.NewEncoder(w).Encode(map[string]any{"errno": 0, "has_more": true, "list": []teraFile{{ID: 1, Name: "container", Path: "/container", IsDir: 1}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"errno": 0, "has_more": false, "list": []teraFile{}})
	}))
	defer server.Close()
	client := teraClient{baseURL: server.URL, client: server.Client()}
	entries, complete, err := client.scan(context.Background(), teraSession{Cookie: "ndus=test", BDSToken: "bds"}, "/", 10)
	if err != nil || !complete || len(entries) != 1 || calls < 2 {
		t.Fatalf("%+v %v %v calls=%d", entries, complete, err, calls)
	}
}

func TestKoromanActuallyRomanizesKoreanWithoutTranslation(t *testing.T) {
	names, err := romanizeKorean(context.Background(), []string{"지연2", "민주", "가을", "션", "수아", "신위"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Jiyeon2", "Minju", "Gaeul", "Syeon", "Sua", "Sinwi"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("%d got %q want %q", i, names[i], w)
		}
	}
}

func TestTeraBoxOmitsAbsentBrowserTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("bdstoken") {
			t.Error("Empty bdstoken was sent despite being absent in the browser request")
		}
		if r.URL.Query().Get("jsToken") != "browser-token" {
			t.Error("Missing browser jsToken")
		}
		w.Write([]byte(`{"errno":0,"list":[]}`))
	}))
	defer server.Close()
	client := teraClient{baseURL: server.URL, client: server.Client()}
	if _, err := client.list(context.Background(), teraSession{Cookie: "ndus=test", JSToken: "browser-token"}, "/", 1); err != nil {
		t.Fatal(err)
	}
}

func TestTeraBoxSessionRejectionIsNotReportedAsGatewayOrAdminLoginFailure(t *testing.T) {
	store := teraTestStore(t)
	key := "test-administrator-key-at-least-24-characters"
	if err := store.saveTeraAccount(context.Background(), key, teraAccount{ID: 1, Label: "First", Root: "/"}, teraSession{Cookie: "ndus=private-session", JSToken: "private-js"}); err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = responseTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"errno":-6,"list":[]}`)), Request: req}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	client := newTeraClient()
	_, err := client.list(context.Background(), teraSession{Cookie: "ndus=private-session"}, "/", 1)
	var rejected *teraSessionError
	if !errors.As(err, &rejected) {
		t.Fatalf("expected session error, got %v", err)
	}
	router := Router(store, key)
	req := httptest.NewRequest("POST", "/api/admin/terabox/preview", strings.NewReader(`{"accountId":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "TERABOX_SESSION_REJECTED") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private-session") || strings.Contains(response.Body.String(), "private-js") {
		t.Fatal("credentials leaked")
	}
}

func TestTeraBoxUsesOnlyTheSelectedTrustedBrowserHost(t *testing.T) {
	for _, host := range []string{"", "www.terabox.com", "dm.terabox.com", "evil.example", "dm.terabox.com@evil.example", "https://dm.terabox.com"} {
		t.Run(host, func(t *testing.T) {
			called := false
			client := newTeraClient()
			client.client.Transport = responseTransport(func(req *http.Request) (*http.Response, error) {
				called = true
				expected := host
				if expected == "" {
					expected = "www.terabox.com"
				}
				if req.URL.Host != expected || req.URL.Scheme != "https" || req.Header.Get("Referer") != "https://"+expected+"/main" {
					t.Errorf("unexpected provider origin %s", req.URL.Host)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"errno":0,"list":[]}`)), Request: req}, nil
			})
			_, err := client.list(context.Background(), teraSession{Host: host, Cookie: "ndus=test"}, "/", 1)
			valid := host == "" || host == "www.terabox.com" || host == "dm.terabox.com"
			if valid && (err != nil || !called) {
				t.Fatalf("trusted host failed: %v", err)
			}
			if !valid && (err == nil || called) {
				t.Fatal("untrusted host reached transport")
			}
		})
	}
}

func TestTeraBoxBrowserHostSurvivesAccountSaveWithoutReturningCredentials(t *testing.T) {
	store := teraTestStore(t)
	key := "test-administrator-key-at-least-24-characters"
	router := Router(store, key)
	for _, host := range []string{"dm.terabox.com", "evil.example"} {
		req := httptest.NewRequest("PUT", "/api/admin/terabox/accounts/1", strings.NewReader(`{"label":"First","root":"/","host":"`+host+`","cookie":"ndus=private-session","jsToken":"private-js"}`))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		expected := 200
		if host == "evil.example" {
			expected = 400
		}
		if recorder.Code != expected {
			t.Fatalf("save host %s status %d: %s", host, recorder.Code, recorder.Body.String())
		}
	}
	session, err := store.teraSession(context.Background(), key, 1)
	if err != nil || session.Host != "dm.terabox.com" {
		t.Fatalf("stored host=%q error=%v", session.Host, err)
	}
	req := httptest.NewRequest("GET", "/api/admin/terabox/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	body := recorder.Body.String()
	if recorder.Code != 200 || !strings.Contains(body, `"host":"dm.terabox.com"`) || strings.Contains(body, "private-session") || strings.Contains(body, "private-js") {
		t.Fatalf("account metadata response: %s", body)
	}
}

func TestTeraBoxRecentFoldersIncludeUpdatesToOlderFolders(t *testing.T) {
	now := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	old := now.Add(-4 * 24 * time.Hour).Unix()
	recent := now.Add(-3 * time.Hour).Unix()
	files := []teraFile{
		{ID: 1, Path: "/261017 하람", IsDir: 1, Created: old, Modified: recent},
		{ID: 2, Path: "/old", IsDir: 1, Created: old, Modified: old},
		{ID: 3, Path: "/missing-creation", IsDir: 1, Modified: recent},
		{ID: 4, Path: "/missing-times", IsDir: 1},
		{ID: 5, Path: "/photo.jpg", Modified: recent},
		{ID: 6, Path: "/boundary", IsDir: 1, Created: old, Modified: now.Add(-72 * time.Hour).Unix()},
		{ID: 7, Path: "/future", IsDir: 1, Created: old, Modified: now.Add(time.Hour).Unix()},
	}
	got, warnings := recentTeraFolders(files, now)
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 3 || got[2].ID != 6 || len(warnings) != 1 {
		t.Fatalf("recent=%+v warnings=%v", got, warnings)
	}
}
