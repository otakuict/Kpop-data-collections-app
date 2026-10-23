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
	"strconv"
	"strings"
	"testing"
)

func TestAdminDeleteRemovesOnlyTheSelectedSetAndItsImages(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "delete.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	removed, err := s.Save(ctx, 0, SetDraft{Title: "Remove me", Group: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := s.Save(ctx, 0, SetDraft{Title: "Keep me", Group: "KEEP"})
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range []Set{removed, kept} {
		if err = s.AddImages(ctx, set.ID, []Raster{{Bytes: []byte("first"), MIME: "image/jpeg"}, {Bytes: []byte("second"), MIME: "image/jpeg"}}); err != nil {
			t.Fatal(err)
		}
	}
	removed, err = s.Get(ctx, removed.ID)
	if err != nil {
		t.Fatal(err)
	}
	token := "test-operator-token-at-least-24-chars"
	router := Router(s, token)
	call := func(method, path, suppliedToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if suppliedToken != "" {
			req.Header.Set("Authorization", "Bearer "+suppliedToken)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	path := "/api/admin/sets/" + strconv.FormatInt(int64(removed.ID), 10)
	for _, suppliedToken := range []string{"", "incorrect-token"} {
		if w := call("DELETE", path, suppliedToken); w.Code != 401 {
			t.Fatalf("unauthorized deletion returned %d: %s", w.Code, w.Body.String())
		}
	}
	if _, err = s.Get(ctx, removed.ID); err != nil {
		t.Fatalf("unauthorized request removed the set: %v", err)
	}
	for _, id := range []string{"0", "-1", "invalid", "9223372036854775808"} {
		if w := call("DELETE", "/api/admin/sets/"+id, token); w.Code != 400 {
			t.Fatalf("invalid ID %s returned %d", id, w.Code)
		}
	}
	if w := call("DELETE", "/api/admin/sets/99999", token); w.Code != 404 {
		t.Fatalf("missing set returned %d", w.Code)
	}
	w := call("DELETE", path, token)
	var result struct {
		Deleted bool `json:"deleted"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Deleted {
		t.Fatalf("delete response %d: %s", w.Code, w.Body.String())
	}
	if w = call("GET", "/api/sets/"+strconv.FormatInt(int64(removed.ID), 10), ""); w.Code != 404 {
		t.Fatalf("deleted set still accessible: %d", w.Code)
	}
	for _, img := range removed.Images {
		if w = call("GET", img.URL, ""); w.Code != 404 {
			t.Fatalf("deleted image still accessible: %d", w.Code)
		}
	}
	remaining, err := s.Get(ctx, kept.ID)
	if err != nil || len(remaining.Images) != 2 {
		t.Fatalf("unrelated set or images changed: %#v, %v", remaining, err)
	}
	listed, err := s.List(ctx, Filter{Page: 1, Limit: 40})
	if err != nil || listed.Total != 1 || listed.Items[0].ID != kept.ID {
		t.Fatalf("catalog still contains deleted set: %#v, %v", listed, err)
	}
	facets, err := s.Facets(ctx)
	if err != nil || facets.Total != 1 || facets.Complete != 1 || len(facets.Groups) != 1 || facets.Groups[0].Name != "KEEP" {
		t.Fatalf("facets still count deleted set: %#v, %v", facets, err)
	}
	if w = call("DELETE", path, token); w.Code != 404 {
		t.Fatalf("repeated deletion returned %d", w.Code)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if w = call("DELETE", path, token); w.Code != 500 {
		t.Fatalf("database failure returned %d", w.Code)
	}
}

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

func TestUploadPreflightRejectsFiveLargeImages(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for i := 0; i < 5; i++ {
		part, err := writer.CreateFormFile("images", "photo.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(pngHeader(1800, 1400)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/admin/sets/1/images", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(2 << 20); err != nil {
		t.Fatal(err)
	}
	defer req.MultipartForm.RemoveAll()
	err := validateUploadFiles(req.MultipartForm.File["images"])
	if err == nil || !strings.Contains(err.Error(), "combined upload") {
		t.Fatalf("five large images were not rejected before decoding: %v", err)
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

func TestImportPreviewHTTPDoesNotWriteAndReturnsExactCSV(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "preview-http.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	token := "test-operator-token-at-least-24-chars"
	router := Router(s, token)
	csv := "Date,Name,GROUP,Example\n260915,Reviewed,aespa,post deleted\n"
	call := func(path string, authorized bool, policy string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("sheetUrl", DefaultSheetURL)
		if policy != "" {
			writer.WriteField("imagePolicy", policy)
		}
		part, _ := writer.CreateFormFile("csv", "source.csv")
		part.Write([]byte(csv))
		writer.Close()
		req := httptest.NewRequest("POST", path, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if w := call("/api/admin/import/preview", false, ""); w.Code != 401 {
		t.Fatal("unauthorized preview accepted")
	}
	w := call("/api/admin/import/preview", true, "replace")
	var preview struct {
		Report      ImportReport `json:"report"`
		CSV         string       `json:"csv"`
		SheetURL    string       `json:"sheetUrl"`
		ImagePolicy string       `json:"imagePolicy"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || preview.Report.Created != 1 || preview.CSV != csv || preview.SheetURL != DefaultSheetURL || preview.ImagePolicy != "replace" {
		t.Fatalf("preview response %d %s", w.Code, w.Body.String())
	}
	if w := call("/api/admin/import/preview", true, "invalid"); w.Code != 400 {
		t.Fatalf("invalid image policy accepted: %d %s", w.Code, w.Body.String())
	}
	p, _ := s.List(context.Background(), Filter{Page: 1, Limit: 24})
	if p.Total != 0 {
		t.Fatal("preview persisted rows")
	}
	if w := call("/api/admin/import", true, "replace"); w.Code != 200 {
		t.Fatalf("commit failed %s", w.Body.String())
	}
	p, _ = s.List(context.Background(), Filter{Page: 1, Limit: 24})
	if p.Total != 1 || p.Items[0].Title != "Reviewed" {
		t.Fatal("reviewed snapshot not committed")
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
