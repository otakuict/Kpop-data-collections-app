package gallery

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Explicit opt-in only: this makes a real, billable TypeSafe request. No TeraBox
// session or Google Sheet is touched; inputs are public example artist names.
func TestTeraLiveJevNames(t *testing.T) {
	if os.Getenv("GALLERY_LIVE_JEV") != "1" {
		t.Skip("set GALLERY_LIVE_JEV=1 to call TypeSafe")
	}
	finder := exampleFinderFromEnv()
	if finder.apiKey == "" {
		t.Fatal("TYPESAFE_API_KEY is required")
	}
	s := teraTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var seed struct {
		Groups map[string][]artistMember `json:"groups"`
	}
	if err := json.Unmarshal(artistRosterSeed, &seed); err != nil {
		t.Fatal(err)
	}
	for group := range seed.Groups {
		if _, err := s.Save(ctx, 0, SetDraft{Title: "Catalog initialization", Group: group}); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := s.artistCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []struct{ original, path, wantGroup, wantName string }{
		{"신위", "/TripleS/261011 신위", "TripleS", "Xinyu"},
		{"쯔위", "/TWICE/261011 쯔위", "TWICE", "Tzuyu"},
		{"가을", "/IVE/261011 가을", "IVE", "Gaeul"},
		{"이지우", "/TripleS/261011 이지우", "TripleS", "Lee Jiwoo"},
		{"김체원", "/LE SSERAFIM/261011 김체원", "LE SSERAFIM", "Kim Chaewon"},
	}
	names := []string{}
	for _, input := range inputs {
		names = append(names, input.original)
	}
	phonetic, err := romanizeKorean(ctx, names)
	if err != nil {
		t.Fatal(err)
	}
	preview := teraPreview{Rows: []teraPreviewRow{}}
	for i, input := range inputs {
		preview.Rows = append(preview.Rows, teraPreviewRow{Original: input.original, Path: input.path, RawName: input.original, Romanized: phonetic[i], SetDate: "2026-10-11", Status: "review"})
	}
	service := teraService{ai: finder}
	if err = service.resolveWithJev(ctx, &preview, catalogMembers(catalog)); err != nil {
		t.Fatal(err)
	}
	for i, row := range preview.Rows {
		confidence := 0.0
		if row.Confidence != nil {
			confidence = *row.Confidence
		}
		t.Logf("%s -> %s | group=%s | KOROMAN=%s | confidence=%.3f | status=%s", inputs[i].original, row.Title, row.Group, row.Romanized, confidence, row.Status)
		if row.Group != inputs[i].wantGroup {
			t.Errorf("%s: expected group %s", inputs[i].original, inputs[i].wantGroup)
		}
		if row.Title != "261011 "+inputs[i].wantName {
			t.Errorf("%s: expected %s", inputs[i].original, inputs[i].wantName)
		}
	}
}
