package gallery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func pngHeader(width, height uint32) []byte {
	var header bytes.Buffer
	header.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	binary.Write(&header, binary.BigEndian, uint32(13))
	header.WriteString("IHDR")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8], ihdr[9] = 8, 2
	header.Write(ihdr)
	binary.Write(&header, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
	return header.Bytes()
}

func TestManualUploadRejectsLargeDimensionsBeforeDecode(t *testing.T) {
	// DecodeConfig needs only a PNG signature and IHDR, so this avoids
	// allocating a giant test image while checking the decode guard.
	_, err := normalizeImage(pngHeader(4000, 3250), "", maxUploadImagePixels)
	if err == nil || !strings.Contains(err.Error(), "12 megapixels") {
		t.Fatalf("large upload was not rejected before decoding: %v", err)
	}
}

func qualityTestJPEG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: c.R + uint8(x), G: c.G + uint8(y), B: c.B, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestNormalizeImagePreservesSuitablePublicJPEG(t *testing.T) {
	source := "https://lh3.googleusercontent.com/photo"
	original := qualityTestJPEG(t, color.RGBA{R: 20, G: 30, B: 40, A: 255})
	raster, err := NormalizeImage(original, source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raster.Bytes, original) || raster.MIME != "image/jpeg" || raster.SourceURL != source {
		t.Fatal("small public JPEG was recompressed or its metadata changed")
	}
}

func TestImageRequestURLUsesFullGooglePreview(t *testing.T) {
	for input, want := range map[string]string{
		"https://lh3.googleusercontent.com/photo":       "https://lh3.googleusercontent.com/photo=s1024",
		"https://lh3.googleusercontent.com/photo=s1024": "https://lh3.googleusercontent.com/photo=s1024",
		"https://media.ganknow.com/public/photo.jpg":    "https://media.ganknow.com/public/photo.jpg",
	} {
		got, err := imageRequestURL(input)
		if err != nil || got.String() != want {
			t.Fatalf("imageRequestURL(%q) = %v, %v; want %q", input, got, err, want)
		}
	}
}

func TestRefreshImageFetchesFullGoogleVariantAndKeepsID(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "refresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	set, err := s.Save(ctx, 0, SetDraft{Title: "Preview", Group: "aespa", Example: "https://ganknow.com/post/example"})
	if err != nil {
		t.Fatal(err)
	}
	source := "https://lh3.googleusercontent.com/photo"
	old := qualityTestJPEG(t, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	newImage := qualityTestJPEG(t, color.RGBA{R: 90, G: 20, B: 30, A: 255})
	other := qualityTestJPEG(t, color.RGBA{R: 50, G: 40, B: 30, A: 255})
	if err := s.AddImages(ctx, set.ID, []Raster{{Bytes: old, MIME: "image/jpeg", SourceURL: source}, {Bytes: other, MIME: "image/jpeg"}}); err != nil {
		t.Fatal(err)
	}
	set, err = s.Get(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, beforeURL := set.Images[0].ID, set.Images[0].URL
	ids, err := LowResolutionGoogleImages(ctx, s)
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("legacy Google preview selection: %v, %v", ids, err)
	}
	originalClient := fetchClient
	defer func() { fetchClient = originalClient }()
	requested := ""
	fetchClient = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		requested = r.URL.String()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/jpeg"}}, Body: io.NopCloser(bytes.NewReader(newImage)), Request: r}, nil
	})}
	if err := RefreshImage(ctx, s, id); err != nil {
		t.Fatal(err)
	}
	if requested != source+"=s1024" {
		t.Fatalf("fetched %q, want sized source", requested)
	}
	data, mime, err := s.ImageBytes(ctx, id)
	if err != nil || !bytes.Equal(data, newImage) || mime != "image/jpeg" {
		t.Fatalf("refreshed bytes or MIME incorrect: %s, %v", mime, err)
	}
	set, err = s.Get(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if set.Images[0].ID != id || set.Images[0].URL == beforeURL || set.Images[0].SourceURL != source {
		t.Fatal("refresh did not preserve the image identity and update its cache URL")
	}
}

func TestReloadSetImagesRefreshesFullAlbum(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "full-album.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	set, err := s.Save(ctx, 0, SetDraft{Title: "Full album", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	source := "https://lh3.googleusercontent.com/full-album-photo"
	rasters := []Raster{}
	for i := 0; i < MaxSetImages; i++ {
		r := Raster{Bytes: qualityTestJPEG(t, color.RGBA{R: uint8(i * 30), G: 20, B: 30, A: 255}), MIME: "image/jpeg"}
		if i == 0 {
			r.SourceURL = source
		}
		rasters = append(rasters, r)
	}
	if err := s.AddImages(ctx, set.ID, rasters); err != nil {
		t.Fatal(err)
	}
	set, err = s.Get(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, oldURL := set.Images[0].ID, set.Images[0].URL
	replacement := qualityTestJPEG(t, color.RGBA{R: 220, G: 20, B: 30, A: 255})
	originalClient := fetchClient
	defer func() { fetchClient = originalClient }()
	requests := 0
	fetchClient = &http.Client{Transport: responseTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/jpeg"}}, Body: io.NopCloser(bytes.NewReader(replacement)), Request: r}, nil
	})}
	set, err = ReloadSetImages(ctx, s, set.ID)
	if err != nil || len(set.Images) != MaxSetImages || requests != 1 || set.Images[0].ID != id || set.Images[0].URL == oldURL {
		t.Fatalf("full album was not reloaded: requests=%d, set=%#v, err=%v", requests, set, err)
	}
	data, _, err := s.ImageBytes(ctx, id)
	if err != nil || !bytes.Equal(data, replacement) {
		t.Fatalf("replacement image not stored: %v", err)
	}
	manual, err := s.Save(ctx, 0, SetDraft{Title: "Manual album", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReloadSetImages(ctx, s, manual.ID); !errors.Is(err, ErrNoImageSource) {
		t.Fatalf("manual album reload error = %v", err)
	}
}
