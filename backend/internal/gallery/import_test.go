package gallery

import (
	"strings"
	"testing"
)

func TestSheetDecorationsAndUnknownDateArePreserved(t *testing.T) {
	sets, err := ParseSheet(strings.NewReader("banner,,,,,\nDate,Name,GROUP,Source,anything,Example,REMARK\n260915,Karina,aespa,kdata,,https://ganknow.com/post/test,limited\n0,Unknown,IVE,,,post deleted,note\n"), "sheet:test")
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 || sets[0].Date != "2026-09-15" || sets[1].Date != "" || sets[1].Example != "post deleted" {
		t.Fatalf("unexpected sets: %#v", sets)
	}
	again, _ := ParseSheet(strings.NewReader("Date,Name,GROUP,Source,anything,Example,REMARK\n260915,Karina,aespa,kdata,,https://ganknow.com/post/test,limited\n"), "sheet:test")
	if again[0].ImportKey != sets[0].ImportKey {
		t.Fatal("identity depends on row number")
	}
}

func TestSheetRejectsInvalidDateWithoutDroppingOtherRows(t *testing.T) {
	sets, err := ParseSheet(strings.NewReader("Date,Name,GROUP,Source,anything,Example,REMARK\n260231,Invalid,aespa,,,,\n260915,Valid,aespa,,,,\n"), "sheet:test")
	if err != nil || len(sets) != 2 || sets[0].Date != "" || sets[0].ImportWarning == "" {
		t.Fatalf("lost row or invalid date accepted: %#v %v", sets, err)
	}
}

func TestExtractOriginalPreviewAndIgnoreAvatar(t *testing.T) {
	urls := ExtractImages(`<meta content="https://media.ganknow.com/private/a/photo.jpg=w1200-h627-c" property="og:image"><script>var avatar="https:\u002F\u002Fmedia.ganknow.com\u002Fpublic\u002Favatar.jpg"</script>`, "https://ganknow.com/post/test")
	if len(urls) != 1 || urls[0] != "https://media.ganknow.com/private/a/photo.jpg" {
		t.Fatalf("bad preview: %#v", urls)
	}
}

func TestFetchURLRejectsLocalAndCredentials(t *testing.T) {
	for _, raw := range []string{"http://example.com/x", "https://127.0.0.1/x", "https://[::1]/x", "https://user:password@example.com/x", "https://example.com:8443/x"} {
		if _, err := ParseRemoteURL(raw); err == nil {
			t.Fatalf("accepted unsafe URL %s", raw)
		}
	}
}
