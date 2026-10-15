package gallery

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRibbonFollowsTenMostRecentlyAddedSets(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ribbons.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var first Set
	for i := 0; i < newRibbonLimit; i++ {
		set, err := s.Save(ctx, 0, SetDraft{Title: fmt.Sprintf("Set %d", i), Group: "IVE", Date: "2026-10-01"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = set
		}
	}
	first, err = s.Get(ctx, first.ID)
	if err != nil || !first.Style.New {
		t.Fatalf("tenth most recent should still be New: %#v, %v", first.Style, err)
	}
	backdated, err := s.Save(ctx, 0, SetDraft{Title: "Backdated addition", Group: "IVE", Date: "2020-01-01"})
	if err != nil || !backdated.Style.New {
		t.Fatalf("newly added backdated set should be New: %#v, %v", backdated.Style, err)
	}
	first, err = s.Get(ctx, first.ID)
	if err != nil || first.Style.New {
		t.Fatalf("eleventh most recent kept New ribbon: %#v, %v", first.Style, err)
	}
	first, err = s.SaveStyled(ctx, first.ID, SetDraft{Title: first.Title, Group: first.Group, Date: first.Date}, &SetStyle{New: true, Wishlist: true})
	if err != nil || first.Style.New || !first.Style.Wishlist {
		t.Fatalf("manually rechecked old New ribbon was not cleared: %#v, %v", first.Style, err)
	}
	page, err := s.List(ctx, Filter{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	foundBackdated := false
	for _, set := range page.Items {
		if set.ID == backdated.ID {
			foundBackdated = true
			if !set.Style.New {
				t.Fatal("creation-order ribbon was lost because the set sorts by an older date")
			}
		}
	}
	if !foundBackdated {
		t.Fatal("backdated set was not in the result page")
	}
	for i := 0; i < newRibbonLimit; i++ {
		if _, err := s.Save(ctx, 0, SetDraft{Title: fmt.Sprintf("Later %d", i), Group: "IVE"}); err != nil {
			t.Fatal(err)
		}
	}
	backdated, err = s.Get(ctx, backdated.ID)
	if err != nil || backdated.Style.New {
		t.Fatalf("backdated addition kept its ribbon after ten newer additions: %#v, %v", backdated.Style, err)
	}
}

func TestNewRibbonExpiresForBulkImportAndLegacyFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ribbons.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	drafts := make([]SetDraft, newRibbonLimit+1)
	for i := range drafts {
		drafts[i] = SetDraft{Title: fmt.Sprintf("Imported %d", i), Group: "aespa", ImportKey: fmt.Sprintf("sheet:%d", i)}
	}
	if _, err = s.PreviewImport(ctx, drafts); err != nil {
		t.Fatal(err)
	}
	if report, err := s.Import(ctx, drafts); err != nil || report.Created != len(drafts) {
		t.Fatalf("bulk import: %#v, %v", report, err)
	}
	first, err := s.Get(ctx, 1)
	if err != nil || first.Style.New {
		t.Fatalf("bulk import left eleventh ribbon: %#v, %v", first.Style, err)
	}
	latest, err := s.Get(ctx, SetID(len(drafts)))
	if err != nil || !latest.Style.New {
		t.Fatalf("bulk import did not mark latest set New: %#v, %v", latest.Style, err)
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sets SET style='{"new":true,"custom":{"keep":1}}' WHERE id=?`, first.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err = s.Get(ctx, first.ID)
	if err != nil || first.Style.New {
		t.Fatalf("old persisted ribbon survived reopening: %#v, %v", first.Style, err)
	}
	var encoded string
	if err = s.db.QueryRowContext(ctx, "SELECT style FROM sets WHERE id=?", first.ID).Scan(&encoded); err != nil || !strings.Contains(encoded, `"custom":{"keep":1}`) {
		t.Fatalf("unrelated style data was lost: %q, %v", encoded, err)
	}
}

func TestSetStyleMigrationAndImportPreservesEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE sets (id INTEGER PRIMARY KEY, title TEXT NOT NULL, group_name TEXT NOT NULL, date TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '', example TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', raw_date TEXT NOT NULL DEFAULT '', raw_cells TEXT NOT NULL DEFAULT '{}', import_key TEXT UNIQUE, import_warning TEXT NOT NULL DEFAULT '', origin TEXT NOT NULL DEFAULT 'manual', image_state TEXT NOT NULL DEFAULT 'pending', image_error TEXT NOT NULL DEFAULT ''); INSERT INTO sets(title,group_name) VALUES('Existing','aespa');`)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	existing, err := s.Get(ctx, 1)
	if err != nil || existing.Style.New || existing.Style.Wishlist || existing.Style.Color != "" {
		t.Fatalf("legacy style: %#v, %v", existing.Style, err)
	}
	created, err := s.SaveStyled(ctx, 0, SetDraft{Title: "New set", Group: "aespa"}, &SetStyle{New: true, Wishlist: true, Color: "#AABBCC"})
	if err != nil || !created.Style.New || !created.Style.Wishlist || created.Style.Color != "#aabbcc" {
		t.Fatalf("created style: %#v, %v", created.Style, err)
	}
	_, err = s.Save(ctx, created.ID, SetDraft{Title: "Renamed", Group: "aespa"})
	if err != nil {
		t.Fatal(err)
	}
	created, err = s.Get(ctx, created.ID)
	if err != nil || !created.Style.Wishlist || created.Style.Color != "#aabbcc" {
		t.Fatalf("metadata edit lost style: %#v, %v", created.Style, err)
	}
	if _, err = s.SaveStyled(ctx, created.ID, SetDraft{Title: "Bad", Group: "aespa"}, &SetStyle{Color: "red"}); err == nil {
		t.Fatal("invalid card color accepted")
	}
	drafts := []SetDraft{{Title: "Imported", Group: "IVE", ImportKey: "sheet:1"}}
	report, err := s.Import(ctx, drafts)
	if err != nil || report.Created != 1 {
		t.Fatalf("import: %#v, %v", report, err)
	}
	imported, err := s.Get(ctx, report.Candidates[0])
	if err != nil || !imported.Style.New {
		t.Fatalf("new import style: %#v, %v", imported.Style, err)
	}
	_, err = s.SaveStyled(ctx, imported.ID, SetDraft{Title: imported.Title, Group: imported.Group}, &SetStyle{Wishlist: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Import(ctx, drafts)
	if err != nil {
		t.Fatal(err)
	}
	imported, err = s.Get(ctx, imported.ID)
	if err != nil || imported.Style.New || !imported.Style.Wishlist {
		t.Fatalf("reimport replaced card style: %#v, %v", imported.Style, err)
	}
}

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
	r, err = s.ImportWithImagePolicy(ctx, drafts, ReplaceChangedImages)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.List(ctx, Filter{Group: "aespa", Page: 1, Limit: 24})
	if len(p.Items[0].Images) != 0 || p.Items[0].ImageState != "pending" {
		t.Fatal("stale image survived changed example")
	}
}
func TestImportKeepsImagesWhenExampleChangesByDefault(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "keep-images.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	drafts, err := ParseSheet(strings.NewReader("ID,Date,Name,GROUP,Example\nold,260915,Old,aespa,https://example.com/old\n"), "sheet:test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Import(ctx, drafts)
	if err != nil {
		t.Fatal(err)
	}
	id := first.ScanCandidates[0]
	if err := s.AddImages(ctx, id, []Raster{{Bytes: []byte("first"), MIME: "image/jpeg"}, {Bytes: []byte("second"), MIME: "image/jpeg"}}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	drafts[0].Title = "Updated"
	drafts[0].Example = "https://example.com/new"
	preview, err := s.PreviewImport(ctx, drafts)
	if err != nil || len(preview.ScanCandidates) != 0 {
		t.Fatalf("keep preview: %#v, %v", preview, err)
	}
	replacePreview, err := s.PreviewImportWithImagePolicy(ctx, drafts, ReplaceChangedImages)
	if err != nil || len(replacePreview.ScanCandidates) != 1 || replacePreview.ScanCandidates[0] != id {
		t.Fatalf("replace preview: %#v, %v", replacePreview, err)
	}
	result, err := s.Import(ctx, drafts)
	if err != nil || len(result.ScanCandidates) != 0 {
		t.Fatalf("keep import: %#v, %v", result, err)
	}
	after, err := s.Get(ctx, id)
	if err != nil || after.Title != "Updated" || after.Example != drafts[0].Example || after.ImageState != "ready" || len(after.Images) != 2 || after.Images[0].ID != before.Images[0].ID || after.Images[1].ID != before.Images[1].ID {
		t.Fatalf("existing images were not preserved: %#v, %v", after, err)
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
	changed, err := s.ImportWithImagePolicy(ctx, drafts, ReplaceChangedImages)
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
	preview, err := s.PreviewImportWithImagePolicy(ctx, drafts, ReplaceChangedImages)
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
	committed, err := s.ImportWithImagePolicy(ctx, drafts, ReplaceChangedImages)
	if err != nil || committed.Created != preview.Created || committed.Changed != preview.Changed {
		t.Fatalf("commit differs: %#v %v", committed, err)
	}
	x, _ := s.Get(ctx, id)
	if x.Title != "Renamed" || len(x.Images) != 0 {
		t.Fatal("confirm did not apply reviewed changes")
	}
}
