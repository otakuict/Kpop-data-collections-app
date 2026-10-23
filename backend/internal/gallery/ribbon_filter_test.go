package gallery

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestRibbonFilterUsesExistingStyleFlagsBeforePagination(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ribbon-filter.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, fixture := range []struct {
		title, group string
		style        SetStyle
	}{
		{"New aespa", "aespa", SetStyle{New: true}},
		{"Selective aespa", "aespa", SetStyle{Selective: true}},
		{"Both ribbons", "aespa", SetStyle{New: true, Selective: true}},
		{"New IVE", "IVE", SetStyle{New: true}},
		{"Trade ribbons", "aespa", SetStyle{Wishlist: true, NoTrade: true}},
		{"No ribbons", "aespa", SetStyle{}},
	} {
		if _, err := s.SaveStyled(context.Background(), 0, SetDraft{Title: fixture.title, Group: fixture.group}, &fixture.style); err != nil {
			t.Fatal(err)
		}
	}
	router := Router(s, "test-operator-token-at-least-24-chars")
	for _, fixture := range []struct {
		query string
		total int
		check func(SetStyle) bool
	}{
		{"ribbon=new&group=aespa&limit=1&page=2", 2, func(style SetStyle) bool { return style.New }},
		{"ribbon=selective&group=aespa&limit=1", 2, func(style SetStyle) bool { return style.Selective }},
		{"ribbon=wishlist&group=aespa", 1, func(style SetStyle) bool { return style.Wishlist }},
		{"ribbon=noTrade&group=aespa", 1, func(style SetStyle) bool { return style.NoTrade }},
	} {
		t.Run(fixture.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/api/sets?"+fixture.query, nil))
			var page Page
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != fixture.total || len(page.Items) != 1 {
				t.Fatalf("wrong filtered page %d: %s", w.Code, w.Body.String())
			}
			if page.Items[0].Group != "aespa" || !fixture.check(page.Items[0].Style) {
				t.Fatalf("returned a set without the selected ribbon: %#v", page.Items[0])
			}
		})
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/sets?ribbon=invalid", nil))
	if w.Code != 400 {
		t.Fatalf("invalid ribbon returned %d: %s", w.Code, w.Body.String())
	}
}
