package gallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImageGrabberRequestsInternalAPIAndSelectsGalleryImages(t *testing.T) {
	const example = "https://ganknow.com/post/example"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/extract" || r.URL.Query().Get("url") != example {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"sourceUrl": example, "images": []map[string]any{
			{"type": "image", "renderedUrl": "https://lh3.googleusercontent.com/photo=s300", "originalUrl": "https://lh3.googleusercontent.com/photo=s0", "thumbUrl": "https://lh3.googleusercontent.com/photo=s100"},
			{"type": "image", "renderedUrl": "https://media.ganknow.com/public/PM.one.jpg"},
			{"type": "image", "originalUrl": "https://media.ganknow.com/public/PM.one.jpg"},
			{"type": "video", "renderedUrl": "https://media.ganknow.com/public/movie.mp4"},
			{"type": "image", "renderedUrl": "http://127.0.0.1/private.jpg"},
		}})
	}))
	defer server.Close()
	t.Setenv("GANK_IMAGE_GRABBER_URL", server.URL)
	urls, err := fetchGankGrabberImages(context.Background(), 1, example)
	if err != nil || len(urls) != 2 || urls[0] != "https://lh3.googleusercontent.com/photo=s1200" || urls[1] != "https://media.ganknow.com/public/PM.one.jpg" {
		t.Fatalf("unexpected images: %#v %v", urls, err)
	}
}

func TestImageGrabberRejectsErrorsAndMismatchedPages(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"unavailable", `{"error":"extract failed"}`, 500},
		{"no images", `{"error":"No image media found on the page."}`, 422},
		{"invalid JSON", `<html>Bad Gateway</html>`, 200},
		{"wrong post", `{"sourceUrl":"https://ganknow.com/post/other","images":[]}`, 200},
		{"empty images", `{"sourceUrl":"https://ganknow.com/post/a","images":[]}`, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); w.Write([]byte(test.body)) }))
			defer server.Close()
			t.Setenv("GANK_IMAGE_GRABBER_URL", server.URL)
			_, err := fetchGankGrabberImages(context.Background(), 1, "https://ganknow.com/post/a")
			if err == nil {
				t.Fatal("expected extraction error")
			}
		})
	}
}

func TestGrabberMediaPacingIsSeparateFromGankPages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "pacing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now()
	if _, err := store.reserveRemote(ctx, "ganknow.com", now, GankInterval); err != nil {
		t.Fatal(err)
	}
	media := remoteRequestKey("media.ganknow.com")
	if media == "ganknow.com" {
		t.Fatal("media still shares the page/API schedule")
	}
	if wait, err := store.reserveRemote(ctx, media, now, time.Second); err != nil || wait != 0 {
		t.Fatalf("media waited on page request: %s %v", wait, err)
	}
	if wait, err := store.reserveRemote(ctx, media, now.Add(100*time.Millisecond), time.Second); err != nil || wait < 800*time.Millisecond {
		t.Fatalf("media pacing missing: %s %v", wait, err)
	}
	if remoteRequestKey("api.ganknow.com") != "ganknow.com" {
		t.Fatal("API must retain Gank pacing")
	}
}

func TestImageGrabberSimulation(t *testing.T) {
	base := os.Getenv("GALLERY_GRABBER_SIM_URL")
	if base == "" {
		t.Skip("set GALLERY_GRABBER_SIM_URL to the local Postman simulation")
	}
	t.Setenv("GANK_IMAGE_GRABBER_URL", base)
	for _, suffix := range []string{"example", "empty", "unavailable"} {
		urls, err := fetchGankGrabberImages(context.Background(), 1, "https://ganknow.com/post/"+suffix)
		if suffix == "example" {
			if err != nil || len(urls) != 2 || !strings.HasSuffix(urls[0], "=s1200") {
				t.Fatalf("simulation: %#v %v", urls, err)
			}
		} else if err == nil {
			t.Fatalf("expected %s error", suffix)
		}
	}
}

func TestImageGrabberPreservesSignedURLsAndRejectsUnsupportedHosts(t *testing.T) {
	signed := "https://gank-media.bucket.r2.cloudflarestorage.com/public/a%2Bb.jpg?X-Amz-Signature=example&X-Amz-Expires=86400"
	if got := grabberImageURL(signed); got != signed {
		t.Fatalf("signed URL changed: %s", got)
	}
	for _, raw := range []string{"https://evil.test/image.jpg", "https://media.ganknow.com.evil.test/image.jpg", "https://127.0.0.1/image.jpg", "javascript:alert(1)"} {
		if got := grabberImageURL(raw); got != "" {
			t.Fatalf("accepted unsafe URL: %s", got)
		}
	}
}

func TestImageGrabberLiveExtraction(t *testing.T) {
	base := os.Getenv("GALLERY_GRABBER_LIVE_URL")
	if base == "" {
		t.Skip("set GALLERY_GRABBER_LIVE_URL for an explicit live smoke test")
	}
	t.Setenv("GANK_IMAGE_GRABBER_URL", base)
	store, err := Open(filepath.Join(t.TempDir(), "live.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	example := os.Getenv("GALLERY_GRABBER_LIVE_EXAMPLE")
	if example == "" {
		example = googleGankExample
	}
	set, err := store.Save(ctx, 0, SetDraft{Title: "Live grabber smoke test", Group: "S2IT", Example: example})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExtractTo(ctx, store, set.ID, 2)
	if err != nil || len(result.Images) != 2 || result.ImageState != "ready" {
		t.Fatalf("live extraction: %#v %v", result, err)
	}
	for _, img := range result.Images {
		if strings.Contains(img.SourceURL, "lh3.googleusercontent.com") && !strings.HasSuffix(img.SourceURL, "=s1200") {
			t.Fatalf("wrong image variant: %s", img.SourceURL)
		}
	}
}

func TestMediaCooldownStopsFurtherDownloads(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cooldown.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.pauseRemote(ctx, remoteRequestKey("media.ganknow.com"), time.Now().Add(time.Hour), "HTTP 429"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	transport := &remoteTransport{store: store, base: responseTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })}
	request, _ := http.NewRequestWithContext(ctx, "GET", "https://media.ganknow.com/public/PM.one.jpg", nil)
	_, err = transport.RoundTrip(request)
	if !isRemotePaused(err) || calls != 0 {
		t.Fatalf("download escaped media cooldown: %d %v", calls, err)
	}
}

func TestImageIdentityIgnoresGoogleSizesAndR2SignatureRenewal(t *testing.T) {
	for _, pair := range [][2]string{
		{"https://lh3.googleusercontent.com/photo", "https://lh3.googleusercontent.com/photo=s1200"},
		{"https://lh3.googleusercontent.com/photo=s1024", "https://lh3.googleusercontent.com/photo=s1200"},
		{"https://gank-media.bucket.r2.cloudflarestorage.com/photo.jpg?X-Amz-Signature=old", "https://gank-media.bucket.r2.cloudflarestorage.com/photo.jpg?X-Amz-Signature=new&X-Amz-Date=20261026"},
	} {
		if imageSourceKey(pair[0]) != imageSourceKey(pair[1]) {
			t.Fatalf("same photo has different identities: %v", pair)
		}
	}
	if imageSourceKey("https://lh3.googleusercontent.com/one=s1200") == imageSourceKey("https://lh3.googleusercontent.com/two=s1200") {
		t.Fatal("different photos share an identity")
	}
}
