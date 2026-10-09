package gallery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

const googleGankExample = "https://ganknow.com/post/e63b82e9-3d38-4245-beed-84974767969e"
const googleGankDocument = `<meta property="og:image" content="https://lh3.googleusercontent.com/first=w1200-h627-c">
<meta name="twitter:image" content="https://lh3.googleusercontent.com/first=w1200-h627-c">
<div id="e63b82e9-3d38-4245-beed-84974767969e">
<img src="https://lh3.googleusercontent.com/avatar" alt="Avatar of IdolLove">
<img src="https://lh3.googleusercontent.com/border">
<img src="https://lh3.googleusercontent.com/first=s1024" alt="SEUNGBI - IdolLove">
<img src="https://lh3.googleusercontent.com/second=s1024" alt="SEUNGBI - IdolLove">
<img src="https://lh3.googleusercontent.com/third=s1024" alt="SEUNGBI - IdolLove">
</div>
<div id="another-post"><img src="https://lh3.googleusercontent.com/unrelated=s1024" alt="SEUNGBI - IdolLove"></div>
<script>window.__NUXT__={postMedia:[],blurUrl:"https:\u002F\u002Flh3.googleusercontent.com\u002Fblur",avatar:"https:\u002F\u002Flh3.googleusercontent.com\u002Favatar"};</script>`

func TestGankExtractionReadsGoogleHostedImagesOnlyFromCurrentPost(t *testing.T) {
	urls := ExtractImages(googleGankDocument, googleGankExample)
	if len(urls) != 3 || urls[0] != "https://lh3.googleusercontent.com/first" || urls[1] != "https://lh3.googleusercontent.com/second" || urls[2] != "https://lh3.googleusercontent.com/third" {
		t.Fatalf("wrong Google-hosted post images: %#v", urls)
	}
}

const googleGankSSRDocument = `<meta property="og:image" content="https://lh3.googleusercontent.com/first=w1200-h627-c">
<script>window.__NUXT__=(function(g,i,m,n,o,p,q,r){return {data:[{post:{id:g,accessType:"public",postMedia:[{url:m,type:i,blurUrl:p},{url:n,type:i,thumbUrl:p},{url:o,type:i},{url:q,type:"video"}],author:{avatar:p}}},{post:{id:"another-post",accessType:"public",postMedia:[{url:r,type:i}]}}]}}("e63b82e9-3d38-4245-beed-84974767969e","image","https:\u002F\u002Flh3.googleusercontent.com\u002Ffirst","https:\u002F\u002Flh3.googleusercontent.com\u002Fsecond","https:\u002F\u002Flh3.googleusercontent.com\u002Fthird","https:\u002F\u002Flh3.googleusercontent.com\u002Favatar","https:\u002F\u002Flh3.googleusercontent.com\u002Fvideo","https:\u002F\u002Flh3.googleusercontent.com\u002Funrelated"));</script>`

func TestGankExtractionResolvesGoogleImageReferencesFromServerHTML(t *testing.T) {
	urls := ExtractImages(googleGankSSRDocument, googleGankExample)
	if len(urls) != 3 || urls[1] != "https://lh3.googleusercontent.com/second" || urls[2] != "https://lh3.googleusercontent.com/third" {
		t.Fatalf("Nuxt post images not resolved: %#v", urls)
	}
	private := strings.ReplaceAll(googleGankSSRDocument, `accessType:"public"`, `accessType:"private"`)
	if urls := ExtractImages(private, googleGankExample); len(urls) != 1 {
		t.Fatalf("non-public post yielded additional images: %#v", urls)
	}
}
