package gallery

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSheetSyncUpdatesOnlyEditedCellsAndKeepsImportIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "sheet-sync.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, source, _ := SheetURL(DefaultSheetURL)
	initial := "banner,,,,,,\nDate,Name,GROUP,Source,anything,Example,REMARK\n260915,Old,aespa,Studio,keep,https://example.com/old,note\n"
	drafts, err := ParseSheet(strings.NewReader(initial), source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Import(ctx, drafts)
	if err != nil {
		t.Fatal(err)
	}
	id := report.Candidates[0]
	old, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.ImportKey(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var written []struct {
		Range  string     `json:"range"`
		Values [][]string `json:"values"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 3600})
		case r.Header.Get("Authorization") != "Bearer test-token":
			w.WriteHeader(401)
		case strings.HasSuffix(r.URL.Path, "/values:batchUpdate"):
			var payload struct {
				ValueInputOption string `json:"valueInputOption"`
				Data             []struct {
					Range  string     `json:"range"`
					Values [][]string `json:"values"`
				} `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload.ValueInputOption != "RAW" {
				t.Errorf("value input option = %q", payload.ValueInputOption)
			}
			written = payload.Data
			w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/values/'Collection'"):
			json.NewEncoder(w).Encode(map[string]any{"values": [][]string{{"banner"}, {"Date", "Name", "GROUP", "Source", "anything", "Example", "REMARK"}, {"260915", "Old", "aespa", "Studio", "keep", "https://example.com/old", "note"}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Collection"}}}})
		}
	}))
	defer server.Close()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	syncer := &SheetSync{spreadsheetID: "test", gid: 0, source: source, email: "test@example.com", privateKey: privateKey,
		client: server.Client(), apiBase: server.URL, tokenURL: server.URL + "/token", tokenExpires: time.Time{}}
	requested := old.SetDraft
	requested.Title = "Renamed"
	requested.Source = "New Studio"
	updated, synced, err := syncer.UpdateExisting(ctx, store, old, requested, key)
	if err != nil || !synced {
		t.Fatalf("sync: %v, %v", synced, err)
	}
	if len(written) != 2 {
		t.Fatalf("changed cells: %#v", written)
	}
	changed := map[string]string{}
	for _, cell := range written {
		changed[cell.Range] = cell.Values[0][0]
	}
	if changed["'Collection'!B3"] != "Renamed" || changed["'Collection'!D3"] != "New Studio" {
		t.Fatalf("wrong cells: %#v", changed)
	}
	if _, err = store.SaveStyled(ctx, id, updated, nil); err != nil {
		t.Fatal(err)
	}
	again, err := ParseSheet(strings.NewReader("banner,,,,,,\nDate,Name,GROUP,Source,anything,Example,REMARK\n260915,Renamed,aespa,New Studio,keep,https://example.com/old,note\n"), source)
	if err != nil {
		t.Fatal(err)
	}
	reimport, err := store.Import(ctx, again)
	if err != nil || reimport.Created != 0 || reimport.Updated != 1 || reimport.Unchanged != 1 {
		t.Fatalf("reimport after rename: %#v, %v", reimport, err)
	}
}

func TestSheetSyncRejectsChangedCell(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "sheet-conflict.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, source, _ := SheetURL(DefaultSheetURL)
	drafts, _ := ParseSheet(strings.NewReader("Date,Name,GROUP,Source\n260915,Old,aespa,Studio\n"), source)
	report, err := store.Import(ctx, drafts)
	if err != nil {
		t.Fatal(err)
	}
	id := report.Candidates[0]
	old, _ := store.Get(ctx, id)
	key, _ := store.ImportKey(ctx, id)
	writeCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/values:batchUpdate") {
			writeCount++
			w.Write([]byte(`{}`))
			return
		}
		if strings.Contains(r.URL.Path, "/values/") {
			json.NewEncoder(w).Encode(map[string]any{"values": [][]string{{"Date", "Name", "GROUP", "Source"}, {"260915", "Old", "aespa", "Changed in Sheet"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Collection"}}}})
	}))
	defer server.Close()
	syncer := &SheetSync{spreadsheetID: "test", source: source, client: server.Client(), apiBase: server.URL, token: "test-token", tokenExpires: time.Now().Add(time.Hour)}
	requested := old.SetDraft
	requested.Source = "Backoffice edit"
	_, _, err = syncer.UpdateExisting(ctx, store, old, requested, key)
	if !errors.Is(err, ErrSheetConflict) || writeCount != 0 {
		t.Fatalf("conflict: %v; writes=%d", err, writeCount)
	}
}

func TestSheetSyncInsertNewUsesSetDateOrderAndSheetColumns(t *testing.T) {
	for _, test := range []struct {
		name, group, title string
		row                int
	}{
		{"newest in existing group", "TWICE", "261021 Sana", 3},
		{"existing group case", "twice", "261021 Sana", 3},
		{"between existing dates", "TWICE", "260930 Sana", 4},
		{"new group at bottom", "IVE", "261021 Rei", 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := Open(filepath.Join(t.TempDir(), "insert.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			_, source, _ := SheetURL(DefaultSheetURL)
			live := [][]string{
				{"banner"},
				{"Date", "Name", "GROUP", "Source", "extra", "Example", "REMARK"},
				{"261005", "261004 Nayeon", "TWICE", "Studio A", "keep A", "https://example.com/a", "old A"},
				{"260924", "260923 Mina", "TWICE", "Studio B", "keep B", "https://example.com/b", "old B"},
				{"260901", "260831 Karina", "aespa", "Studio C", "keep C", "https://example.com/c", "old C"},
			}
			var batch map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, ":batchUpdate"):
					if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Error(err)
					}
					w.Write([]byte(`{}`))
				case strings.Contains(r.URL.Path, "/values/"):
					json.NewEncoder(w).Encode(map[string]any{"values": live})
				default:
					json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Collection"}}}})
				}
			}))
			defer server.Close()
			syncer := &SheetSync{spreadsheetID: "test", gid: 0, source: source, client: server.Client(), apiBase: server.URL, token: "test-token", tokenExpires: time.Now().Add(time.Hour)}
			setDate, _ := time.Parse("2006-01-02", "2026-10-21")
			if test.name == "between existing dates" {
				setDate, _ = time.Parse("2006-01-02", "2026-09-30")
			}
			draft := SetDraft{Title: test.title, Group: test.group, Date: "2026-10-20", Source: "New Studio", Example: "https://example.com/new", Notes: "Wish list; New"}
			inserted, row, err := syncer.InsertNew(ctx, store, draft, setDate)
			if err != nil {
				t.Fatal(err)
			}
			if row != test.row || inserted.RawDate != "261020" || inserted.Date != draft.Date || inserted.ImportKey == "" {
				t.Fatalf("inserted row=%d, draft=%+v", row, inserted)
			}
			requestRows := batch["requests"].([]any)
			if len(requestRows) != 4 {
				t.Fatalf("requests=%#v", requestRows)
			}
			dimension := requestRows[0].(map[string]any)["insertDimension"].(map[string]any)["range"].(map[string]any)
			if int(dimension["startIndex"].(float64)) != row-1 {
				t.Fatalf("dimension=%#v", dimension)
			}
			for i, kind := range []string{"PASTE_FORMAT", "PASTE_DATA_VALIDATION"} {
				paste := requestRows[i+1].(map[string]any)["copyPaste"].(map[string]any)
				if paste["pasteType"] != kind {
					t.Fatalf("paste=%#v", paste)
				}
			}
			update := requestRows[3].(map[string]any)["updateCells"].(map[string]any)
			values := update["rows"].([]any)[0].(map[string]any)["values"].([]any)
			writtenGroup := test.group
			if test.group == "twice" {
				writtenGroup = "TWICE"
			}
			want := []string{"261020", test.title, writtenGroup, "New Studio", "", "https://example.com/new", "Wish list; New"}
			for i, cell := range values {
				got := cell.(map[string]any)["userEnteredValue"].(map[string]any)["stringValue"]
				if got != want[i] {
					t.Fatalf("column %d = %v, want %q", i, got, want[i])
				}
			}
			saved, err := store.SaveStyled(ctx, 0, inserted, &SetStyle{New: true, Wishlist: true})
			if err != nil || saved.Origin != "sheet" {
				t.Fatalf("local save: %+v, %v", saved, err)
			}
		})
	}
}

func TestSheetSyncInsertNewKeepsPhysicalRowsWithBlankLines(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "insert.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, source, _ := SheetURL(DefaultSheetURL)
	live := [][]string{
		{"banner"},
		{}, // Blank row before the header must still count as a Sheet row.
		{"view mode"},
		{"Date", "Name", "GROUP", "Source", "anything", "Example", "REMARK"},
		{"261021", "261018 Leeseo", "IVE", "Studio A", "", "https://example.com/a", ""},
		{"261021", "261018 Leeseo", "IVE", "Studio B", "", "https://example.com/b", ""},
		{"261019", "261004 Rei", "IVE", "Studio C", "", "https://example.com/c", ""},
		{}, // A second blank row must not shift later Sheet row numbers either.
		{"261019", "261004 Sana", "TWICE", "Studio D", "", "https://example.com/d", ""},
	}
	parsed, err := parseLiveRows(live, source)
	if err != nil {
		t.Fatal(err)
	}
	if parsed[0].Number != 5 || parsed[3].Number != 9 {
		t.Fatalf("physical row numbers: first=%d last=%d", parsed[0].Number, parsed[3].Number)
	}
	var batch map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Error(err)
			}
			w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "/values/"):
			json.NewEncoder(w).Encode(map[string]any{"values": live})
		default:
			json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Collection"}}}})
		}
	}))
	defer server.Close()
	syncer := &SheetSync{spreadsheetID: "test", gid: 0, source: source, client: server.Client(), apiBase: server.URL, token: "test-token", tokenExpires: time.Now().Add(time.Hour)}
	setDate, _ := time.Parse("2006-01-02", "2026-10-18")
	draft := SetDraft{Title: "261018 Leeseo", Group: "IVE", Date: "2026-10-22", Source: "Studio New", Example: "https://example.com/new"}
	inserted, row, err := syncer.InsertNew(ctx, store, draft, setDate)
	if err != nil {
		t.Fatal(err)
	}
	if row != 7 || inserted.ImportKey == "" || batch == nil {
		t.Fatalf("row=%d, key=%q, batch=%v", row, inserted.ImportKey, batch)
	}
}
