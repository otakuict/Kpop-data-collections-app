package gallery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func teraTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "tera.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTeraCatalogUsesGalleryGroupsAndKeepsArtistIdentitiesSeparate(t *testing.T) {
	s := teraTestStore(t)
	ctx := context.Background()
	for _, g := range []string{"TripleS", "NMIXX", "MEOVV", "SOLO"} {
		if _, err := s.Save(ctx, 0, SetDraft{Title: "Existing", Group: g}); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := s.artistCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 4 {
		t.Fatalf("Invented groups: %+v", catalog)
	}
	members := catalogMembers(catalog)
	found := map[string]string{}
	for _, m := range members {
		for _, name := range []string{"지연2", "김나경", "주빈", "안나", "쯔위"} {
			if suffix, ok := artistSuffix(name, m); ok {
				found[name] = m.Name + suffix
			}
		}
	}
	if found["지연2"] != "JiYeon2" || found["김나경"] != "Kim Nakyoung" || found["주빈"] != "JooBin" || found["안나"] != "Anna" || found["쯔위"] != "" {
		t.Fatalf("Incorrect identity or invented roster: %+v", found)
	}
	jiwoo := 0
	for _, m := range members {
		if _, ok := artistSuffix("지우", m); ok {
			jiwoo++
		}
	}
	if jiwoo != 2 {
		t.Fatalf("Expected two separate Jiwoo memberships, got %d", jiwoo)
	}
	before, _ := s.List(ctx, Filter{Page: 1, Limit: 40})
	if err = s.seedArtistCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := s.List(ctx, Filter{Page: 1, Limit: 40})
	if before.Total != after.Total {
		t.Fatal("Catalog seeding changed gallery")
	}
	catalog2, _ := s.artistCatalog(ctx)
	if len(catalogMembers(catalog2)) != len(members) {
		t.Fatal("Catalog seeding duplicated members")
	}
}

func TestTeraPreviewScansPrivateDriveButDoesNotCreateSets(t *testing.T) {
	s := teraTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	if _, err := s.Save(ctx, 0, SetDraft{Title: "Existing", Group: "TripleS"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveTeraAccount(ctx, "key", teraAccount{ID: 2, Label: "Second", Root: "/albums"}, teraSession{Cookie: "ndus=private"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "ndus=private" {
			t.Error("Wrong account session")
		}
		json.NewEncoder(w).Encode(map[string]any{"errno": 0, "list": []teraFile{
			{ID: 1, Name: "240404 지연2", Path: "/albums/240404 지연2", IsDir: 1, Created: now.Add(-96 * time.Hour).Unix(), Modified: now.Add(-time.Hour).Unix()},
			{ID: 2, Name: "261024 카에데", Path: "/albums/261024 카에데", IsDir: 1, Created: now.Add(-6 * 24 * time.Hour).Unix()},
		}})
	}))
	defer server.Close()
	service := teraService{store: s, key: "key", provider: teraClient{baseURL: server.URL, client: server.Client()}, now: func() time.Time { return now }}
	preview, err := service.preview(ctx, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Rows) != 1 || preview.Rows[0].Title != "240404 JiYeon2" || preview.Rows[0].Group != "TripleS" || preview.Rows[0].Romanized != "Jiyeon2" || preview.Rows[0].Status != "new" {
		t.Fatalf("%+v", preview)
	}
	if preview.Rows[0].UpdatedAt != now.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("wrong update time: %+v", preview.Rows[0])
	}
	sets, _ := s.List(ctx, Filter{Page: 1, Limit: 40})
	if sets.Total != 1 {
		t.Fatal("Preview wrote gallery records")
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "ndus") {
		t.Fatal("Preview leaked session")
	}
}

func TestTeraJevCanOnlySelectSuppliedDatabaseArtists(t *testing.T) {
	chosen := "artist_99"
	confidence := 0.99
	choiceCount := 2
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Questions map[string]struct {
				Criteria map[string]any `json:"criteria"`
			} `json:"questions"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("Bad JSON")
		}
		criteria := body.Questions["folder_0"].Criteria
		if len(criteria) != choiceCount || criteria["artist_7"] == nil || criteria["none"] == nil {
			t.Error("Choices did not match DB candidates")
		}
		json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"folder_0": map[string]any{"type": "choice", "choice": chosen, "confidence": confidence}}})
	}))
	defer server.Close()
	service := teraService{ai: exampleFinder{apiKey: "test", model: "jev-latest", aiURL: server.URL, client: server.Client()}}
	member := artistMember{ID: 7, Group: "MEOVV", Name: "Anna", Aliases: []string{"안나"}}
	preview := teraPreview{Rows: []teraPreviewRow{{SetDate: "2026-10-13", RawName: "안나", Status: "review"}}}
	if err := service.resolveWithJev(context.Background(), &preview, []artistMember{member}); err == nil {
		t.Fatal("Accepted invented artist")
	}
	chosen = "artist_7"
	confidence = 0.2
	if err := service.resolveWithJev(context.Background(), &preview, []artistMember{member}); err != nil {
		t.Fatal(err)
	}
	if preview.Rows[0].Status != "review" {
		t.Fatal("Low confidence auto-selected")
	}
	confidence = 0.99
	if err := service.resolveWithJev(context.Background(), &preview, []artistMember{member}); err != nil {
		t.Fatal(err)
	}
	if preview.Rows[0].Title != "261013 Anna" || preview.Rows[0].Group != "MEOVV" {
		t.Fatalf("%+v", preview.Rows[0])
	}
	choiceCount = 3
	preview.Rows[0].Status = "review"
	other := artistMember{ID: 8, Group: "Another gallery group", Name: "Anna", Aliases: []string{"안나"}}
	if err := service.resolveWithJev(context.Background(), &preview, []artistMember{member, other}); err != nil {
		t.Fatal(err)
	}
	if preview.Rows[0].Status != "review" {
		t.Fatal("Ambiguous group was automatically selected without a path clue")
	}
}

func TestTeraConfirmWritesSheetAndDBExactlyOnceAndRejectsUnreviewedRows(t *testing.T) {
	s := teraTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, source, _ := SheetURL(DefaultSheetURL)
	csv := "Date,Name,GROUP,Source,Example,REMARK\n261023,260101 Existing,MEOVV,,,\n"
	drafts, err := ParseSheet(strings.NewReader(csv), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Import(ctx, drafts); err != nil {
		t.Fatal(err)
	}
	if err = s.seedArtistCatalog(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.saveTeraAccount(ctx, "key", teraAccount{ID: 1, Label: "First", Root: "/", Source: "studio"}, teraSession{Cookie: "ndus=test"}); err != nil {
		t.Fatal(err)
	}
	var revision string
	s.db.QueryRow("SELECT updated_at FROM terabox_accounts WHERE id=1").Scan(&revision)
	preview := teraPreview{ID: "reviewed", AccountID: 1, Rows: []teraPreviewRow{{FileID: 10, Status: "new"}}}
	payload, _ := json.Marshal(preview)
	if _, err = s.db.Exec("INSERT INTO terabox_previews(id,account_id,account_revision,expires_at,payload) VALUES(?,1,?,?,?)", preview.ID, revision, now.Add(time.Hour).Unix(), string(payload)); err != nil {
		t.Fatal(err)
	}
	writes := 0
	failWrite := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchUpdate"):
			writes++
			if failWrite {
				http.Error(w, "unavailable", 503)
				return
			}
			w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "/values/"):
			json.NewEncoder(w).Encode(map[string]any{"values": [][]string{{"Date", "Name", "GROUP", "Source", "Example", "REMARK"}, {"261023", "260101 Existing", "MEOVV", "", "", ""}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 0, "title": "Collection"}}}})
		}
	}))
	defer server.Close()
	syncer := &SheetSync{spreadsheetID: "test", source: source, client: server.Client(), apiBase: server.URL, token: "test", tokenExpires: now.Add(time.Hour)}
	oldTitle := drafts[0]
	oldTitle.Date = "2099-01-01"
	if _, _, e := syncer.insertNew(ctx, s, oldTitle, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true); !errors.Is(e, ErrSheetConflict) || writes != 0 {
		t.Fatal("TeraBox inserted a duplicate title from an earlier added date")
	}
	service := teraService{store: s, key: "key", now: func() time.Time { return now }}
	seller := "chosen-seller"
	request := teraImportRequest{PreviewID: "reviewed", Rows: []teraImportRow{{FileID: 10, Title: "261013 Anna", Group: "MEOVV", SetDate: "2026-10-13", Source: &seller}}}
	bad := request
	bad.Rows = []teraImportRow{{FileID: 999, Title: "Injected", Group: "MEOVV", SetDate: "2026-10-13"}}
	if _, err = service.commit(ctx, syncer, true, bad); err == nil || writes != 0 {
		t.Fatal("Unreviewed folder was written")
	}
	if _, err = service.commit(ctx, syncer, false, request); err == nil || writes != 0 {
		t.Fatal("Disabled Sheet sync was ignored")
	}
	results, err := service.commit(ctx, syncer, true, request)
	if err != nil || len(results) != 1 || results[0].Status != "saved" || writes != 1 {
		t.Fatalf("%+v writes=%d err=%v", results, writes, err)
	}
	saved, err := s.Get(ctx, results[0].SetID)
	if err != nil || saved.Origin != "sheet" || saved.Source != seller {
		t.Fatalf("%+v %v", saved, err)
	}
	results, err = service.commit(ctx, syncer, true, request)
	if err != nil || results[0].Status != "duplicate" || writes != 1 {
		t.Fatalf("Retried external write: %+v %d %v", results, writes, err)
	}
	// A write with unknown outcome must remain blocked, even after another click.
	preview.Rows[0].FileID = 11
	payload, _ = json.Marshal(preview)
	s.db.Exec("UPDATE terabox_previews SET payload=? WHERE id='reviewed'", string(payload))
	request.Rows[0].FileID = 11
	request.Rows[0].Title = "261014 Ella"
	failWrite = true
	results, err = service.commit(ctx, syncer, true, request)
	if err != nil || results[0].Status != "reconcile" {
		t.Fatalf("%+v %v", results, err)
	}
	count := writes
	results, err = service.commit(ctx, syncer, true, request)
	if err != nil || writes != count || results[0].Status != "reconcile" {
		t.Fatal("Uncertain Sheet write retried")
	}
	results, err = service.recover(ctx, syncer, true, 1)
	if err != nil || len(results) != 1 || results[0].Status != "retry" || writes != count {
		t.Fatalf("Missing-row recovery: %+v %v", results, err)
	}
	if _, err = s.db.Exec("INSERT INTO terabox_imports(account_id,file_id,state,title,group_name) VALUES(1,12,'pending','260101 Existing','MEOVV')"); err != nil {
		t.Fatal(err)
	}
	results, err = service.recover(ctx, syncer, true, 1)
	if err != nil || len(results) != 1 || results[0].Status != "saved" || results[0].SetID == 0 || writes != count {
		t.Fatalf("Existing-row recovery: %+v %v", results, err)
	}
	if _, err = s.db.Exec("UPDATE terabox_previews SET expires_at=?", now.Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = service.commit(ctx, syncer, true, request); err == nil {
		t.Fatal("Expired preview was accepted")
	}
}

func TestTeraBoxTitleCheckUsesFinalSavedTitleAndGroup(t *testing.T) {
	s := teraTestStore(t)
	if _, err := s.Save(context.Background(), 0, SetDraft{Title: "240404 Jiyeon2", Group: "TripleS"}); err != nil {
		t.Fatal(err)
	}
	router := Router(s, "test-administrator-key-at-least-24-characters")
	for _, tc := range []struct {
		query     string
		duplicate bool
	}{
		{"title=Jiyeon2&group=TripleS&setDate=2024-04-04", true},
		{"title=240404%20JIYEON2%20&group=TripleS&setDate=2024-04-04", true},
		{"title=Jiyeon2&group=MEOVV&setDate=2024-04-04", false},
		{"title=Jiyeon3&group=TripleS&setDate=2024-04-04", false},
	} {
		req := httptest.NewRequest("GET", "/api/admin/terabox/title-check?"+tc.query, nil)
		req.Header.Set("Authorization", "Bearer test-administrator-key-at-least-24-characters")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var body struct {
			Duplicate bool `json:"duplicate"`
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Duplicate != tc.duplicate {
			t.Fatalf("query=%s status=%d body=%s", tc.query, response.Code, response.Body.String())
		}
	}
}

func TestTeraBoxRoutesRequireAdminAndNeverEchoCookies(t *testing.T) {
	s := teraTestStore(t)
	router := Router(s, "long-test-admin-token-over-24-characters")
	for _, endpoint := range []string{"/api/admin/terabox/accounts", "/api/admin/artists"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", endpoint, nil))
		if w.Code != 401 {
			t.Fatalf("Unprotected endpoint %s", endpoint)
		}
	}
}

func TestTeraPreviewReusesRootNamesIncludingZIPWithoutReadingSubfolders(t *testing.T) {
	store := teraTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	if _, err := store.Save(ctx, 0, SetDraft{Title: "Existing", Group: "TripleS"}); err != nil {
		t.Fatal(err)
	}
	if err := store.saveTeraAccount(ctx, "key", teraAccount{ID: 2, Label: "Second", Root: "/Albums"}, teraSession{Cookie: "ndus=second"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dir") != "/Albums" || r.Header.Get("Cookie") != "ndus=second" {
			t.Error("wrong root or account")
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(teraPage{Errno: new(int), List: []teraFile{
			{ID: 30, Name: "Limited 260814 Ahyeon", Path: "/Albums/Limited 260814 Ahyeon", IsDir: 1, Modified: now.Add(-2 * time.Hour).Unix()},
			{ID: 10, Name: "240404 지연2.ZIP", Path: "/Albums/240404 지연2.ZIP", Modified: now.Add(-time.Hour).Unix()},
			{ID: 20, Name: "older", Path: "/Albums/older", IsDir: 1, Created: now.Add(-20 * 24 * time.Hour).Unix()},
			{ID: 5, Name: "261024 지연3", Path: "/Albums/261024 지연3", IsDir: 1, Modified: now.Unix()},
		}})
	}))
	defer server.Close()
	service := teraService{store: store, key: "key", now: func() time.Time { return now }, provider: teraClient{baseURL: server.URL, client: server.Client()}}
	names, err := service.names(ctx, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.preview(ctx, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Rows) != len(names.Entries) || preview.Scanned != names.Scanned || preview.Complete != names.Complete {
		t.Fatalf("names=%+v preview=%+v", names, preview)
	}
	for i, row := range preview.Rows {
		if row.FileID != names.Entries[i].FileID || row.Original != names.Entries[i].Name {
			t.Fatalf("names and scan differ at %d", i)
		}
	}
	zip := preview.Rows[1]
	if zip.Title != "240404 JiYeon2" || zip.RawName != "지연2" || zip.Group != "TripleS" {
		t.Fatalf("ZIP parsed incorrectly: %+v", zip)
	}
}
