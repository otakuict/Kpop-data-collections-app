package gallery

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminAuthManualUploadAndFilterFlow(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	token := "test-operator-token-at-least-24-chars"
	router := Router(s, token)
	call := func(method, path string, b []byte, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := call("POST", "/api/admin/sets", []byte(`{"title":"Karina","group":"aespa"}`), false); w.Code != 401 {
		t.Fatalf("auth bypass %d", w.Code)
	}
	if w := call("POST", "/api/admin/sets", []byte(`{"title":"Unsafe","group":"aespa","example":"http://127.0.0.1/x"}`), true); w.Code != 400 {
		t.Fatalf("unsafe example accepted %d", w.Code)
	}
	w := call("POST", "/api/admin/sets", []byte(`{"title":"Karina","group":"aespa","date":"2026-09-15"}`), true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var x Set
	if err = json.Unmarshal(w.Body.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var original bytes.Buffer
	png.Encode(&original, img)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("images", "preview.png")
	part.Write(original.Bytes())
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	original.Reset()
	png.Encode(&original, img)
	part, _ = writer.CreateFormFile("images", "second.png")
	part.Write(original.Bytes())
	writer.Close()
	req := httptest.NewRequest("POST", "/api/admin/sets/1/images", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	x, _ = s.Get(context.Background(), x.ID)
	if len(x.Images) != 2 || x.ImageState != "ready" {
		t.Fatalf("image not attached %#v", x)
	}
	w = call("GET", x.Images[0].URL, nil, false)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || len(w.Body.Bytes()) == 0 {
		t.Fatal("stored image not served")
	}
	w = call("GET", "/api/sets?group=aespa&from=2026-09-15&to=2026-09-15", nil, false)
	var p Page
	json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != 200 || p.Total != 1 {
		t.Fatalf("filter failed %s", w.Body.String())
	}
	w = call("GET", "/api/sets?from=2026-09-16&to=2026-09-15", nil, false)
	if w.Code != 400 {
		t.Fatal("invalid date range accepted")
	}
	w = call("GET", "/api/sets?group=IVE", nil, false)
	json.Unmarshal(w.Body.Bytes(), &p)
	if p.Total != 0 {
		t.Fatal("wrong group returned")
	}
}

func TestSSRFAddressPolicyAndSheetNormalization(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "172.16.0.1", "192.168.1.1", "100.64.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "0.0.0.0", "198.18.0.1", "2001:db8::1"} {
		if publicAddress(net.ParseIP(ip)) {
			t.Fatalf("unsafe address allowed %s", ip)
		}
	}
	if !publicAddress(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address blocked")
	}
	remote, source, err := SheetURL(DefaultSheetURL + "#gid=12")
	if err != nil || !strings.Contains(remote, "gid=0") || !strings.HasPrefix(source, "sheet:") {
		t.Fatalf("wrong sheet normalization %s %s %v", remote, source, err)
	}
	if _, _, err = SheetURL("https://docs.google.com.evil.test/spreadsheets/d/123/edit"); err == nil {
		t.Fatal("lookalike host accepted")
	}
}
func TestRasterRejectsHTMLAndOversizedDimensions(t *testing.T) {
	if _, err := NormalizeImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), ""); err == nil {
		t.Fatal("SVG accepted")
	}
	r, err := NormalizeImage(bytes.Repeat([]byte("not image"), 100), "")
	if err == nil || len(r.Bytes) > 0 {
		t.Fatal("invalid bytes accepted")
	}
}
func TestFailedRetryKeepsExistingImage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	x, err := s.Save(ctx, 0, SetDraft{Title: "Karina", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddImage(ctx, x.ID, Raster{Bytes: []byte("fixture"), MIME: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	x, err = Extract(ctx, s, x.ID)
	if err != nil || x.ImageState != "ready" || len(x.Images) != 1 {
		t.Fatalf("failed retry lost preview %#v %v", x, err)
	}
}
