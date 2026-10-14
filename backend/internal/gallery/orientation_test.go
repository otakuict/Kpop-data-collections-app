package gallery

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"path/filepath"
	"testing"
)

func jpegWithOrientation(t *testing.T, orientation uint16) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	tiff := make([]byte, 26)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], orientation)
	segment := append([]byte("Exif\x00\x00"), tiff...)
	result := append([]byte{}, encoded.Bytes()[:2]...)
	result = append(result, 0xff, 0xe1, 0, byte(len(segment)+2))
	result = append(result, segment...)
	return append(result, encoded.Bytes()[2:]...)
}

func TestNormalizeImageAppliesEXIFOrientation(t *testing.T) {
	for _, orientation := range []uint16{1, 6, 8} {
		input := jpegWithOrientation(t, orientation)
		if got := jpegOrientation(input); got != int(orientation) {
			t.Fatalf("orientation %d read as %d", orientation, got)
		}
		raster, err := NormalizeImage(input, "")
		if err != nil {
			t.Fatal(err)
		}
		config, err := jpeg.DecodeConfig(bytes.NewReader(raster.Bytes))
		if err != nil {
			t.Fatal(err)
		}
		w, h := 3, 2
		if orientation != 1 {
			w, h = 2, 3
		}
		if config.Width != w || config.Height != h {
			t.Fatalf("orientation %d produced %dx%d, want %dx%d", orientation, config.Width, config.Height, w, h)
		}
	}
}

func TestOrientImageRotatesPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	red := color.RGBA{R: 255, A: 255}
	img.Set(0, 0, red)
	for _, tc := range []struct{ orientation, x, y int }{{6, 1, 0}, {8, 0, 2}} {
		got := orientImage(img, tc.orientation)
		if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 3 || got.At(tc.x, tc.y) != red {
			t.Fatalf("orientation %d did not rotate top-left pixel", tc.orientation)
		}
	}
}

func TestRotateStoredImageUpdatesDimensionsAndURL(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rotation.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	set, err := s.Save(ctx, 0, SetDraft{Title: "Portrait", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	first := jpegWithOrientation(t, 1)
	second := jpegWithOrientation(t, 6)
	if err := s.AddImages(ctx, set.ID, []Raster{{Bytes: first, MIME: "image/jpeg"}, {Bytes: second, MIME: "image/jpeg"}}); err != nil {
		t.Fatal(err)
	}
	set, err = s.Get(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := set.Images[0].URL
	if err := s.RotateImage(ctx, set.ID, set.Images[0].ID); err != nil {
		t.Fatal(err)
	}
	set, err = s.Get(ctx, set.ID)
	if err != nil {
		t.Fatal(err)
	}
	if set.Images[0].URL == before {
		t.Fatal("rotated image kept its cached URL")
	}
	data, _, err := s.ImageBytes(ctx, set.Images[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 2 || config.Height != 3 {
		t.Fatalf("rotated image is %dx%d", config.Width, config.Height)
	}
}
