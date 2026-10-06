package gallery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestAlbumLimitDeduplicatesAndRejectsSixthImageAtomically(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "album.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	x, err := s.Save(ctx, 0, SetDraft{Title: "Karina", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	raster := func(n int) Raster { return Raster{Bytes: []byte(fmt.Sprintf("image-%d", n)), MIME: "image/jpeg"} }
	if err = s.AddImages(ctx, x.ID, []Raster{raster(1)}); !errors.Is(err, ErrAlbumIncomplete) {
		t.Fatalf("one-image upload accepted: %v", err)
	}
	x, _ = s.Get(ctx, x.ID)
	if len(x.Images) != 0 {
		t.Fatal("incomplete upload was partially committed")
	}
	if err = s.AddImages(ctx, x.ID, []Raster{raster(1), raster(2)}); err != nil {
		t.Fatal(err)
	}
	if err = s.AddImages(ctx, x.ID, []Raster{raster(3), raster(4), raster(5), raster(6)}); !errors.Is(err, ErrAlbumFull) {
		t.Fatalf("six images accepted: %v", err)
	}
	x, _ = s.Get(ctx, x.ID)
	if len(x.Images) != 2 {
		t.Fatal("oversized album was partially committed")
	}
	if err = s.AddImages(ctx, x.ID, []Raster{raster(3), raster(4), raster(5)}); err != nil {
		t.Fatal(err)
	}
	if err = s.AddImage(ctx, x.ID, raster(5)); err != nil {
		t.Fatalf("duplicate at capacity: %v", err)
	}
	if err = s.AddImage(ctx, x.ID, raster(6)); !errors.Is(err, ErrAlbumFull) {
		t.Fatalf("single image bypassed limit: %v", err)
	}
	f, err := s.Facets(ctx)
	if err != nil || f.Complete != 1 {
		t.Fatalf("complete facets: %#v %v", f, err)
	}
}

func TestGankExtractionReadsDistinctPostImagesWithoutBlurOrAvatar(t *testing.T) {
	doc := `<meta property="og:image" content="https://media.ganknow.com/private/creator/PM.first.jpg=w1200-h627-c"><script>window.__NUXT__={postMedia:[{url:a},{url:b}],avatar:"https:\u002F\u002Fmedia.ganknow.com\u002Fpublic\u002Favatar.jpg",blurUrl:"https:\u002F\u002Fmedia.ganknow.com\u002Fpublic\u002FBP.hidden.blur.png"};var a="https:\u002F\u002Fmedia.ganknow.com\u002Fprivate\u002Fcreator\u002FPM.first.jpg",b="https:\u002F\u002Fmedia.ganknow.com\u002Fprivate\u002Fcreator\u002FPM.second.jpg";</script>`
	urls := ExtractImages(doc, "https://ganknow.com/post/example")
	if len(urls) != 2 || urls[0] != "https://media.ganknow.com/private/creator/PM.first.jpg" || urls[1] != "https://media.ganknow.com/private/creator/PM.second.jpg" {
		t.Fatalf("wrong post images: %#v", urls)
	}
	if urls := ExtractImages(doc, "https://ganknow.com.evil.test/post/example"); len(urls) != 1 {
		t.Fatalf("lookalike treated as Gank: %#v", urls)
	}
}

func TestGumroadExtractionUsesProductCoversAndIgnoresRecommendations(t *testing.T) {
	doc := `<meta property="og:image" content="https://public-files.gumroad.com/first"><div data-page='{"props":{"product":{"covers":[{"url":"https://public-files.gumroad.com/first","type":"image"},{"url":"https://public-files.gumroad.com/second","type":"image"},{"url":"https://public-files.gumroad.com/video","type":"video"}]},"recommended":{"covers":[{"url":"https://public-files.gumroad.com/unrelated","type":"image"}]}}}'></div>`
	urls := ExtractImages(doc, "https://loveshakedata.gumroad.com/l/chaewon")
	if len(urls) != 2 || urls[1] != "https://public-files.gumroad.com/second" {
		t.Fatalf("wrong product covers: %#v", urls)
	}
}
