package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
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
	for _, secret := range []string{"private-value", "hidden-header"} {
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
	if strings.Contains(logs, "private-value") {
		t.Fatalf("logged signed URL query: %s", logs)
	}
}

func TestIncomingLogsQueryRequestAndResponseBodiesWithoutConsumingThem(t *testing.T) {
	output := captureHTTPLogs(t)
	router := gin.New()
	router.Use(httpRequestLogger())
	router.POST("/scan", func(c *gin.Context) {
		var body map[string]any
		if c.ShouldBindJSON(&body) != nil || body["accountId"] != float64(2) {
			t.Error("logger consumed request body")
		}
		c.JSON(502, gin.H{"error": "TeraBox returned HTTP 403", "access_token": "response-secret"})
	})
	req := httptest.NewRequest("POST", "/scan?page=3&jsToken=query-secret", strings.NewReader(`{"accountId":2,"cookie":"ndus=cookie-secret","nested":{"apiKey":"key-secret"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer admin-secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	logs := output.String()
	for _, expected := range []string{"direction=incoming", "query=", "page", "accountId", "TeraBox returned HTTP 403", "body=", "[REDACTED]"} {
		if !strings.Contains(logs, expected) {
			t.Errorf("Missing %s: %s", expected, logs)
		}
	}
	for _, secret := range []string{"query-secret", "cookie-secret", "key-secret", "admin-secret", "response-secret"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !strings.Contains(w.Body.String(), "response-secret") {
		t.Fatal("logger changed the response")
	}
}

func TestOutgoingLogsCompleteExchangeAndPreservesTheResponse(t *testing.T) {
	output := captureHTTPLogs(t)
	client := &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"dir":"/albums","bdstoken":"body-secret"}` {
			t.Errorf("request changed: %s", body)
		}
		return &http.Response{StatusCode: 403, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"errno":-6,"message":"session rejected","jsToken":"echo-secret"}`)), Request: r}, nil
	})}
	req, _ := http.NewRequestWithContext(context.WithValue(context.Background(), requestIDKey{}, "trace-id"), "POST", "https://www.terabox.com/api/list?page=1&jsToken=query-secret", strings.NewReader(`{"dir":"/albums","bdstoken":"body-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "ndus=cookie-secret")
	response, err := loggedDo(client, req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), "echo-secret") {
		t.Fatal("response consumed or modified")
	}
	logs := output.String()
	for _, expected := range []string{"id=trace-id", "direction=outgoing", "call_id=", "query=", "page", "/albums", "status=403", "session rejected", "errno"} {
		if !strings.Contains(logs, expected) {
			t.Errorf("Missing %s: %s", expected, logs)
		}
	}
	for _, secret := range []string{"query-secret", "cookie-secret", "body-secret", "echo-secret"} {
		if strings.Contains(logs, secret) {
			t.Errorf("Leaked %s", secret)
		}
	}
}

func TestHTTPLoggingOmitsLargeJSONAndBinaryBodies(t *testing.T) {
	output := captureHTTPLogs(t)
	for _, fixture := range []struct{ mime, body string }{
		{"application/json", `{"password":"large-secret","padding":"` + strings.Repeat("x", 20000) + `"}`},
		{"image/jpeg", "binary-private-image"},
		{"multipart/form-data; boundary=test", "private-upload-data"},
	} {
		capture := newBodyLog([]byte(fixture.body), len(fixture.body) > httpLogBodyLimit)
		rendered := capture.render(fixture.mime)
		if strings.Contains(rendered, "large-secret") || strings.Contains(rendered, "binary-private-image") || strings.Contains(rendered, "private-upload-data") {
			t.Fatal("unsafe body logged")
		}
	}
	// Ordinary forms must preserve useful input while redacting credentials.
	form := newBodyLog([]byte("dir=%2Falbums&assertion=jwt-secret&password=form-secret"), false).render("application/x-www-form-urlencoded")
	if !strings.Contains(form, "albums") || strings.Contains(form, "jwt-secret") || strings.Contains(form, "form-secret") {
		t.Fatalf("unsafe form: %s", form)
	}
	_ = output
}

func TestLogSanitizerMasksNestedSignedURLs(t *testing.T) {
	raw := []byte(`{"example":"https://example.com/file?page=1&signature=url-secret","session":{"password":"nested-secret"}}`)
	rendered := newBodyLog(raw, false).render("application/json")
	if strings.Contains(rendered, "url-secret") || strings.Contains(rendered, "nested-secret") {
		t.Fatalf("unsafe URL: %s", rendered)
	}
	var data map[string]any
	if json.Unmarshal([]byte(rendered), &data) != nil {
		t.Fatal("rendered JSON invalid")
	}
}
