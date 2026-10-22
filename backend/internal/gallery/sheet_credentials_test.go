package gallery

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func testServiceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "test@example.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})),
		"token_uri":   "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return credentials
}

func TestSheetCredentialUploadRequiresAdminAndActivatesAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gallery.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	token := "test-operator-token-at-least-24-chars"
	credentials := testServiceAccountJSON(t)
	router := Router(store, token)
	upload := func(data []byte, authorized bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("credentials", "google-sheets.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/admin/sheet-credentials", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if response := upload(credentials, false); response.Code != 401 {
		t.Fatalf("unauthorized upload: %d", response.Code)
	}
	if response := upload([]byte(`{"type":"service_account"}`), true); response.Code != 400 {
		t.Fatalf("invalid credential accepted: %d", response.Code)
	}
	if configured, err := store.HasSheetCredentials(context.Background()); err != nil || configured {
		t.Fatalf("invalid upload persisted: %v %v", configured, err)
	}
	response := upload(credentials, true)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"restartRequired":true`) {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("PRIVATE KEY")) {
		t.Fatal("credential returned in response")
	}
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT ciphertext FROM sheet_credentials WHERE id=1`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("PRIVATE KEY")) || bytes.Equal(ciphertext, credentials) {
		t.Fatal("credential stored as plaintext")
	}
	if _, err := store.LoadSheetCredentials(context.Background(), "different-admin-token"); err == nil {
		t.Fatal("wrong administrator token decrypted credential")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	status := httptest.NewRecorder()
	router.ServeHTTP(status, req)
	if !strings.Contains(status.Body.String(), `"sheetSyncRestartRequired":true`) {
		t.Fatalf("pending status: %s", status.Body.String())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.LoadSheetCredentials(context.Background(), token)
	if err != nil || !bytes.Equal(loaded, credentials) {
		t.Fatalf("credential did not survive restart: %v", err)
	}
	syncer, err := NewSheetSyncFromJSON(loaded, DefaultSheetURL)
	if err != nil {
		t.Fatal(err)
	}
	status = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	RouterWithSheetSync(reopened, token, syncer).ServeHTTP(status, req)
	if !strings.Contains(status.Body.String(), `"sheetSyncEnabled":true`) {
		t.Fatalf("restarted status: %s", status.Body.String())
	}
}
