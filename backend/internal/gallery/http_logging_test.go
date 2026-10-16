package gallery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func captureHTTPLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

func TestIncomingHTTPLogsStatusAndRedactsSecrets(t *testing.T) {
	output := captureHTTPLogs(t)
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	req := httptest.NewRequest("GET", "/api/sets?access_token=private-value", nil)
	req.Header.Set("Authorization", "Bearer hidden-header")
	w := httptest.NewRecorder()
	Router(store, "test-operator-token-at-least-24-chars").ServeHTTP(w, req)
	id := w.Header().Get("X-Request-ID")
	logs := output.String()
	if w.Code != 200 || id == "" || !strings.Contains(logs, "http.in.request id="+id) || !strings.Contains(logs, "http.in.response id="+id) || !strings.Contains(logs, "status=200") || !strings.Contains(logs, `route="/api/sets"`) {
		t.Fatalf("missing incoming request details: %s", logs)
	}
	for _, secret := range []string{"private-value", "hidden-header", "access_token"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logged %s: %s", secret, logs)
		}
	}
}

func TestOutgoingHTTPLogsResponseAndRedactsErrorURL(t *testing.T) {
	output := captureHTTPLogs(t)
	ctx := context.WithValue(context.Background(), requestIDKey{}, "parent-request")
	req, err := http.NewRequestWithContext(ctx, "GET", "https://example.com/image.jpg?signature=private-value", nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &loggingTransport{base: responseTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Request: r}, nil
	})}
	response, err := transport.RoundTrip(req)
	if err != nil || response.StatusCode != 503 {
		t.Fatalf("response: %v %v", response, err)
	}
	response.Body.Close()
	transport.base = responseTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("request to " + r.URL.String() + " failed")
	})
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("expected transport failure")
	}
	logs := output.String()
	if !strings.Contains(logs, `http.out.request id=parent-request method=GET url="https://example.com/image.jpg"`) || !strings.Contains(logs, "http.out.response id=parent-request") || !strings.Contains(logs, "status=503") || !strings.Contains(logs, "http.out.error id=parent-request") {
		t.Fatalf("missing outgoing request details: %s", logs)
	}
	if strings.Contains(logs, "private-value") || strings.Contains(logs, "signature") {
		t.Fatalf("logged signed URL query: %s", logs)
	}
}
