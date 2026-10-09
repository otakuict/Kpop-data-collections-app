package gallery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractionCannotAttachImageFromChangedExample(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	x, err := s.Save(ctx, 0, SetDraft{Title: "Karina", Group: "aespa", Example: "https://example.com/old"})
	if err != nil {
		t.Fatal(err)
	}
	oldExample := x.Example
	x, err = s.Save(ctx, x.ID, SetDraft{Title: "Karina", Group: "aespa", Example: "https://example.com/new"})
	if err != nil {
		t.Fatal(err)
	}
	err = s.AddImage(ctx, x.ID, Raster{Bytes: []byte("old image"), MIME: "image/jpeg", expectedExample: &oldExample})
	if err != ErrSourceChanged {
		t.Fatalf("stale source not rejected: %v", err)
	}
	if err = s.Outcome(ctx, x.ID, oldExample, "failed", "old source failed"); err != nil {
		t.Fatal(err)
	}
	x, _ = s.Get(ctx, x.ID)
	if len(x.Images) != 0 || x.ImageState != "pending" || x.ImageError != "" {
		t.Fatalf("stale extraction modified current record %#v", x)
	}
}

func TestImportRepeatsWithoutDuplicatesAndChangedExampleInvalidatesImage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	drafts, _ := ParseSheet(strings.NewReader("Date,Name,GROUP,Example\n260915,Karina,aespa,https://example.com/a\n0,Unknown,IVE,\n"), "sheet:test")
	r, err := s.Import(ctx, drafts)
	if err != nil || r.Created != 2 {
		t.Fatalf("import %#v %v", r, err)
	}
	if err = s.AddImage(ctx, r.Candidates[0], Raster{Bytes: []byte("test-image"), MIME: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	r, err = s.Import(ctx, drafts)
	if err != nil || r.Created != 0 || r.Updated != 2 || len(r.Candidates) != 2 {
		t.Fatalf("duplicate/retry %#v %v", r, err)
	}
	p, err := s.List(ctx, Filter{Group: "aespa", Page: 1, Limit: 24})
	if err != nil || p.Total != 1 || len(p.Items[0].Images) != 1 {
		t.Fatalf("filter/images %#v %v", p, err)
	}
	drafts[0].Example = "https://example.com/b"
	r, err = s.Import(ctx, drafts)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.List(ctx, Filter{Group: "aespa", Page: 1, Limit: 24})
	if len(p.Items[0].Images) != 0 || p.Items[0].ImageState != "pending" {
		t.Fatal("stale image survived changed example")
	}
}
func TestImportRollbackAndSearchLiteralWildcard(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, err = s.Import(ctx, []SetDraft{{Title: "First", Group: "aespa", ImportKey: "key"}, {Title: "Second", Group: "IVE", ImportKey: ""}})
	if err == nil {
		t.Fatal("invalid import identity accepted")
	}
	all, _ := s.List(ctx, Filter{Page: 1, Limit: 24})
	if all.Total != 0 {
		t.Fatal("partial import committed")
	}
	_, err = s.Save(ctx, 0, SetDraft{Title: "Karina", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.List(ctx, Filter{Query: "%", Page: 1, Limit: 24})
	if err != nil || p.Total != 0 {
		t.Fatalf("wildcard was not escaped %v %#v", err, p)
	}
}

func TestSheetScanQueuesOnlyNewOrChangedPublicExamples(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "scan.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	drafts, err := ParseSheet(strings.NewReader("ID,Date,Name,GROUP,Example\nold,260915,Old,aespa,https://example.com/old\nempty,0,Unavailable,IVE,post deleted\n"), "sheet:test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Import(ctx, drafts)
	if err != nil || len(first.ScanCandidates) != 1 {
		t.Fatalf("new public examples: %#v %v", first, err)
	}
	id := first.ScanCandidates[0]
	if err := s.AddImage(ctx, id, Raster{Bytes: []byte("original"), MIME: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	again, err := s.Import(ctx, drafts)
	if err != nil || len(again.ScanCandidates) != 0 || again.Changed != 0 || again.Unchanged != 2 {
		t.Fatalf("unchanged rows queued again: %#v %v", again, err)
	}
	drafts[0].Example = "https://example.com/changed"
	drafts = append(drafts, SetDraft{Title: "New", Group: "IVE", ImportKey: "new", Example: "https://example.com/new"})
	changed, err := s.Import(ctx, drafts)
	if err != nil || changed.Created != 1 || len(changed.ScanCandidates) != 2 || changed.ScanCandidates[0] != id {
		t.Fatalf("changed/new queue: %#v %v", changed, err)
	}
	x, _ := s.Get(ctx, id)
	if len(x.Images) != 0 {
		t.Fatal("changed example retained old images")
	}
	final, err := s.Import(ctx, drafts)
	if err != nil || final.Created != 0 || len(final.ScanCandidates) != 0 {
		t.Fatalf("repeat scan not idempotent: %#v %v", final, err)
	}
}

func TestPreviewImportRollsBackMetadataImagesAndNewRecords(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "preview.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	drafts, _ := ParseSheet(strings.NewReader("ID,Date,Name,GROUP,Example\nold,260915,Old,aespa,https://example.com/old\n"), "sheet:test")
	first, _ := s.Import(ctx, drafts)
	id := first.Candidates[0]
	if err := s.AddImage(ctx, id, Raster{Bytes: []byte("original"), MIME: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	drafts[0].Title = "Renamed"
	drafts[0].Example = "https://example.com/new"
	drafts = append(drafts, SetDraft{Title: "New", Group: "IVE", ImportKey: "new", Example: "https://example.com/new-post"})
	preview, err := s.PreviewImport(ctx, drafts)
	if err != nil || preview.Created != 1 || preview.Changed != 1 || len(preview.Changes) != 2 {
		t.Fatalf("preview %#v %v", preview, err)
	}
	if preview.Changes[0].Before.Title != "Old" || preview.Changes[0].After.Title != "Renamed" {
		t.Fatal("preview lacks old/new values")
	}
	all, _ := s.List(ctx, Filter{Page: 1, Limit: 24})
	if all.Total != 1 || all.Items[0].Title != "Old" || len(all.Items[0].Images) != 1 {
		t.Fatalf("preview wrote to database: %#v", all)
	}
	committed, err := s.Import(ctx, drafts)
	if err != nil || committed.Created != preview.Created || committed.Changed != preview.Changed {
		t.Fatalf("commit differs: %#v %v", committed, err)
	}
	x, _ := s.Get(ctx, id)
	if x.Title != "Renamed" || len(x.Images) != 0 {
		t.Fatal("confirm did not apply reviewed changes")
	}
}
