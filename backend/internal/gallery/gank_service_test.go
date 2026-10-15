package gallery

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

const testServiceURL = "https://ganknow.com/services/111786-kdatastudio-261010-yooyeon-limited"
const testServiceSlug = "111786-kdatastudio-261010-yooyeon-limited"

func TestGankServiceSlugOnlyMatchesServicePages(t *testing.T) {
	for _, test := range []struct {
		url  string
		want bool
	}{
		{testServiceURL, true},
		{testServiceURL + "/?lang=en", true},
		{"https://ganknow.com/post/111786-kdatastudio-261010-yooyeon-limited", false},
		{"https://ganknow.com.evil.test/services/" + testServiceSlug, false},
		{"https://ganknow.com/services/" + testServiceSlug + "/other", false},
		{"https://ganknow.com/services/%2e%2e%2fother", false},
	} {
		_, got := gankServiceSlug(test.url)
		if got != test.want {
			t.Errorf("service match for %q = %v, want %v", test.url, got, test.want)
		}
	}
}

func TestGankServiceImagesSelectsOnlyActiveListingImages(t *testing.T) {
	data := `{"data":{"media":[
		{"type":"image","isActive":true,"url":"https://media.ganknow.com/public/creator/one.jpg"},
		{"type":"video","isActive":true,"url":"https://media.ganknow.com/public/creator/movie.mp4"},
		{"type":"image","isActive":false,"url":"https://media.ganknow.com/public/creator/hidden.jpg"},
		{"type":"image","isActive":true,"url":"https://media.ganknow.com/public/creator/one.jpg"},
		{"type":"image","isActive":true,"url":"https://media.ganknow.com/private/creator/paid.jpg"},
		{"type":"image","isActive":true,"url":"https://evil.test/other.jpg"},
		{"type":"image","isActive":true,"url":"https://lh3.googleusercontent.com/two"}
	]}}`
	urls, err := gankServiceImages([]byte(data))
	if err != nil || len(urls) != 2 || urls[0] != "https://media.ganknow.com/public/creator/one.jpg" || urls[1] != "https://lh3.googleusercontent.com/two" {
		t.Fatalf("listing images: %#v, %v", urls, err)
	}
}

func TestGankServiceOldListingImportsGallery(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "service.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	x, err := s.Save(ctx, 0, SetDraft{Title: "Yooyeon", Group: "TripleS", Example: testServiceURL})
	if err != nil {
		t.Fatal(err)
	}
	oldAPI := "https://api.ganknow.com/v1/catalogs/services/slug/" + testServiceSlug + "/old"
	data := `{"data":{"media":[{"type":"image","isActive":true,"url":"https://lh3.googleusercontent.com/one"},{"type":"image","isActive":true,"url":"https://lh3.googleusercontent.com/two"}]}}`
	if err := s.CachePage(ctx, oldAPI, []byte(data), "application/json"); err != nil {
		t.Fatal(err)
	}
	original := fetchClient
	defer func() { fetchClient = original }()
	requested := []string{}
	fetchClient = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.String())
		if r.URL.Host == "api.ganknow.com" {
			return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"errors":[]}`)), Request: r}, nil
		}
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.RGBA{R: uint8(len(requested) * 50), A: 255})
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(b.Bytes())), Request: r}, nil
	})}
	x, err = ExtractTo(ctx, s, x.ID, 2)
	if err != nil || x.ImageState != "ready" || len(x.Images) != 2 || len(requested) != 3 {
		t.Fatalf("service extraction: state=%s images=%d requests=%#v err=%v", x.ImageState, len(x.Images), requested, err)
	}
	if requested[0] != strings.TrimSuffix(oldAPI, "/old") || requested[1] != "https://lh3.googleusercontent.com/one=s1024" {
		t.Fatalf("unexpected requests: %#v", requested)
	}
}
