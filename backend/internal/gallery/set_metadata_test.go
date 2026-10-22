package gallery

import (
	"testing"
	"time"
)

func TestBuildSetTitleKeepsExistingSetDate(t *testing.T) {
	selected, _ := time.Parse("2006-01-02", "2026-10-22")
	title, setDate := buildSetTitle(selected, "261018 Leeseo datacoffee")
	if title != "261018 Leeseo datacoffee" || setDate.Format("060102") != "261018" {
		t.Fatalf("title=%q setDate=%s", title, setDate.Format("060102"))
	}
	title, setDate = buildSetTitle(selected, "Leeseo datacoffee")
	if title != "261022 Leeseo datacoffee" || !setDate.Equal(selected) {
		t.Fatalf("title=%q setDate=%s", title, setDate.Format("060102"))
	}
}

func TestNotesForBackofficeSaveExcludesNewRibbon(t *testing.T) {
	for _, test := range []struct {
		name  string
		style SetStyle
		notes string
		want  string
	}{
		{"new set", SetStyle{New: true}, "", ""},
		{"legacy New", SetStyle{New: true}, "New", ""},
		{"legacy combination", SetStyle{}, "Wish list; New", "Wish list"},
		{"other ribbon", SetStyle{New: true, NoTrade: true}, "some note", "No trade"},
		{"freeform note", SetStyle{New: true}, "New images expected", "New images expected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := notesForBackofficeSave(&test.style, test.notes); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
