package gallery

import (
	"fmt"
	"strings"
	"time"
)

const MinSetImages = 2
const MaxSetImages = 5

type SetID int64
type ImageID int64

type SetDraft struct {
	Title         string `json:"title"`
	Group         string `json:"group"`
	Date          string `json:"date"`
	Source        string `json:"source"`
	Example       string `json:"example"`
	Notes         string `json:"notes"`
	RawDate       string `json:"rawDate"`
	RawCells      string `json:"rawCells"`
	ImportKey     string `json:"-"`
	ImportWarning string `json:"importWarning"`
}

type Image struct {
	ID        ImageID `json:"id"`
	URL       string  `json:"url"`
	SourceURL string  `json:"sourceUrl"`
}

type Set struct {
	ID SetID `json:"id"`
	SetDraft
	Origin     string  `json:"origin"`
	ImageState string  `json:"imageState"`
	ImageError string  `json:"imageError"`
	Images     []Image `json:"images"`
}

type Filter struct {
	Query, Group, From, To, Sort string
	Page, Limit                  int
}
type Page struct {
	Items []Set `json:"items"`
	Total int   `json:"total"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
}
type Facet struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type Facets struct {
	Groups   []Facet `json:"groups"`
	Total    int     `json:"total"`
	Ready    int     `json:"ready"`
	Complete int     `json:"complete"`
	From     string  `json:"from"`
	To       string  `json:"to"`
}
type ImportReport struct {
	Created        int            `json:"created"`
	Updated        int            `json:"updated"`
	Total          int            `json:"total"`
	Candidates     []SetID        `json:"candidates"`
	ScanCandidates []SetID        `json:"scanCandidates"`
	Changed        int            `json:"changed"`
	Unchanged      int            `json:"unchanged"`
	Changes        []ImportChange `json:"changes"`
}

type ImportChange struct {
	ID     SetID     `json:"id"`
	Kind   string    `json:"kind"`
	Before *SetDraft `json:"before,omitempty"`
	After  SetDraft  `json:"after"`
}

func (d *SetDraft) Validate() error {
	d.Title = strings.TrimSpace(d.Title)
	d.Group = strings.TrimSpace(d.Group)
	if d.Title == "" || d.Group == "" {
		return fmt.Errorf("title and group are required")
	}
	if len(d.Title) > 500 || len(d.Group) > 150 || len(d.Notes) > 10000 || len(d.Example) > 2000 || len(d.Source) > 300 {
		return fmt.Errorf("metadata exceeds allowed length")
	}
	if d.Date != "" {
		if _, err := time.Parse("2006-01-02", d.Date); err != nil {
			return fmt.Errorf("date must be YYYY-MM-DD")
		}
	}
	if strings.HasPrefix(d.Example, "http") {
		if _, err := ParseRemoteURL(d.Example); err != nil {
			return err
		}
	}
	return nil
}
