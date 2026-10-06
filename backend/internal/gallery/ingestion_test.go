package gallery

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type responseTransport func(*http.Request) (*http.Response, error)

func (f responseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGankScheduleSharesSubdomainsAndPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	ctx := context.Background()
	if wait, err := s.reserveRemote(ctx, "ganknow.com", now, 20*time.Second); err != nil || wait != 0 {
		t.Fatalf("first reservation: %v %v", wait, err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if wait, err := s.reserveRemote(ctx, remoteKey("media.ganknow.com"), now.Add(time.Second), 20*time.Second); err != nil || wait < 18*time.Second {
		t.Fatalf("restart/subdomain bypass: %v %v", wait, err)
	}
	until := now.Add(time.Hour)
	if err := s.pauseRemote(ctx, "ganknow.com", until, "HTTP 429"); err != nil {
		t.Fatal(err)
	}
	_, err = s.reserveRemote(ctx, "ganknow.com", now.Add(time.Minute), 20*time.Second)
	var paused *RemotePausedError
	if !errors.As(err, &paused) || paused.Until.Before(until) {
		t.Fatalf("pause missing: %v", err)
	}
}

func TestRateLimitStopsFurtherNetworkRequestsAndHonorsRetryAfter(t *testing.T) {
	for _, status := range []int{429, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			calls := 0
			transport := &remoteTransport{store: s, base: responseTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"172800"}}, Body: io.NopCloser(strings.NewReader("blocked")), Request: r}, nil
			})}
			req, _ := http.NewRequest("GET", "https://ganknow.com/post/test", nil)
			_, err = transport.RoundTrip(req)
			var paused *RemotePausedError
			if !errors.As(err, &paused) || paused.Until.Before(time.Now().Add(47*time.Hour)) {
				t.Fatalf("Retry-After ignored: %v", err)
			}
			req, _ = http.NewRequest("GET", "https://media.ganknow.com/image.jpg", nil)
			_, err = transport.RoundTrip(req)
			if !errors.As(err, &paused) || calls != 1 {
				t.Fatalf("request sent during cooldown: %d %v", calls, err)
			}
		})
	}
}

func TestDocumentCacheAvoidsSecondNetworkRequest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	original := fetchClient
	defer func() { fetchClient = original }()
	calls := 0
	fetchClient = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<html>post</html>")), Request: r}, nil
	})}
	for range 2 {
		b, mime, err := fetchDocument(context.Background(), s, "https://ganknow.com/post/cache")
		if err != nil || string(b) != "<html>post</html>" || mime != "text/html" {
			t.Fatalf("document: %s %s %v", b, mime, err)
		}
	}
	if calls != 1 {
		t.Fatalf("cache missed: %d requests", calls)
	}
}

func TestDailyGankBudgetStopsRequestsAcrossSubdomains(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "budget.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().Truncate(time.Millisecond)
	ctx := context.Background()
	for range 60 {
		if _, err := s.db.Exec("INSERT INTO remote_requests(host,at) VALUES('ganknow.com',?)", now.Add(-time.Hour).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.reserveRemote(ctx, remoteKey("media.ganknow.com"), now, GankInterval)
	var paused *RemotePausedError
	if !errors.As(err, &paused) || !strings.Contains(paused.Reason, "daily") {
		t.Fatalf("daily budget bypassed: %v", err)
	}
}

func TestExtractionStoresMultipleImagesAndSkipsPreviouslyDownloadedURLs(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "extract.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	example := "https://example.com/post"
	x, err := s.Save(ctx, 0, SetDraft{Title: "Multiple photos", Group: "aespa", Example: example})
	if err != nil {
		t.Fatal(err)
	}
	doc := `<meta property="og:image" content="https://example.com/one.jpg"><meta property="og:image" content="https://example.com/two.jpg"><meta property="og:image" content="https://example.com/three.jpg">`
	if err = s.CachePage(ctx, example, []byte(doc), "text/html"); err != nil {
		t.Fatal(err)
	}
	original := fetchClient
	defer func() { fetchClient = original }()
	calls := 0
	fetchClient = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		img := image.NewRGBA(image.Rect(0, 0, 8, 8))
		shade := uint8(calls * 70)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				img.Set(x, y, color.RGBA{R: shade, A: 255})
			}
		}
		var b bytes.Buffer
		png.Encode(&b, img)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(b.Bytes())), Request: r}, nil
	})}
	x, err = ExtractTo(ctx, s, x.ID, 2)
	if err != nil || len(x.Images) != 2 || calls != 2 {
		t.Fatalf("first album: %d images, %d requests, %v", len(x.Images), calls, err)
	}
	x, err = ExtractTo(ctx, s, x.ID, 3)
	if err != nil || len(x.Images) != 3 || calls != 3 {
		t.Fatalf("repeat fetched previous URLs: %d images, %d requests, %v", len(x.Images), calls, err)
	}
}

func TestGankReservationWaitsForInFlightRequest(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "serial.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	if _, err = s.reserveRemote(ctx, "ganknow.com", now, GankInterval); err != nil {
		t.Fatal(err)
	}
	wait, err := s.reserveRemote(ctx, "ganknow.com", now.Add(25*time.Second), GankInterval)
	if err != nil || wait <= 0 {
		t.Fatalf("overlapping request allowed: %v %v", wait, err)
	}
	if err = s.releaseRemote(ctx, "ganknow.com", now.Add(25*time.Second)); err != nil {
		t.Fatal(err)
	}
	wait, err = s.reserveRemote(ctx, "ganknow.com", now.Add(30*time.Second), GankInterval)
	if err != nil || wait < 14*time.Second {
		t.Fatalf("completion gap missing: %v %v", wait, err)
	}
}

func TestGankHostAliasesShareTheSameBudget(t *testing.T) {
	for _, host := range []string{"GANKNOW.COM", "media.ganknow.com.", "ganknow.com."} {
		if remoteKey(host) != "ganknow.com" {
			t.Fatalf("alias bypassed budget: %s", host)
		}
	}
}
