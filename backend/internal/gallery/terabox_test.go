package gallery

import (
	"context"
	"encoding/json"
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
