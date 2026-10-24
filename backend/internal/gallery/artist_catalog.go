package gallery

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

//go:embed assets/artist-roster.json
var artistRosterSeed []byte

type artistMember struct {
	ID      int64    `json:"id"`
	Group   string   `json:"group"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Source  string   `json:"source"`
}
type artistGroup struct {
	Name    string         `json:"name"`
	Kind    string         `json:"kind"`
	Members []artistMember `json:"members"`
}

// The running gallery defines the group universe. Existing sets are untouched;
// public profile facts only enrich members within those exact group names.
func (s *Store) seedArtistCatalog(ctx context.Context) error {
	var seed struct {
		Groups map[string][]artistMember `json:"groups"`
	}
	if err := json.Unmarshal(artistRosterSeed, &seed); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artist_groups(name,kind) SELECT DISTINCT group_name,CASE WHEN upper(group_name)='SOLO' THEN 'solo' ELSE 'group' END FROM sets`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT name FROM artist_groups")
	if err != nil {
		return err
	}
	groups := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			break
		}
		groups = append(groups, name)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, group := range groups {
		for _, member := range seed.Groups[group] {
			aliases, _ := json.Marshal(member.Aliases)
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO artist_members(group_name,name,aliases,source) VALUES(?,?,?,?)`, group, member.Name, string(aliases), member.Source); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) artistCatalog(ctx context.Context) ([]artistGroup, error) {
	if err := s.seedArtistCatalog(ctx); err != nil {
		return nil, err
	}
	result := []artistGroup{}
	// COALESCE keeps groups with no supplied roster visible and editable.
	rows, err := s.db.QueryContext(ctx, `SELECT g.name,g.kind,COALESCE(m.id,0),COALESCE(m.name,''),COALESCE(m.aliases,'[]'),COALESCE(m.source,'') FROM artist_groups g LEFT JOIN artist_members m ON m.group_name=g.name ORDER BY g.name COLLATE NOCASE,m.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var group, kind, aliases string
		var m artistMember
		if err = rows.Scan(&group, &kind, &m.ID, &m.Name, &aliases, &m.Source); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].Name != group {
			result = append(result, artistGroup{Name: group, Kind: kind, Members: []artistMember{}})
		}
		if m.ID == 0 {
			continue
		}
		m.Group = group
		if err = json.Unmarshal([]byte(aliases), &m.Aliases); err != nil {
			return nil, err
		}
		result[len(result)-1].Members = append(result[len(result)-1].Members, m)
	}
	return result, rows.Err()
}

func normalizedArtistName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, norm.NFC.String(name))
}
func memberAliases(m artistMember) []string {
	aliases := append([]string{m.Name}, m.Aliases...)
	words := strings.Fields(m.Name)
	if len(words) > 1 {
		aliases = append(aliases, strings.Join(words[1:], " "))
		for _, alias := range m.Aliases {
			runes := []rune(alias)
			if len(runes) == 3 && runes[0] >= '가' && runes[0] <= '힣' && runes[1] >= '가' && runes[1] <= '힣' && runes[2] >= '가' && runes[2] <= '힣' {
				aliases = append(aliases, string(runes[1:]))
			}
		}
	}
	return aliases
}

// Match the longest artist alias at the beginning, preserving version numbers,
// premium and other title suffixes. Never replace substrings inside another name.
func artistSuffix(name string, m artistMember) (string, bool) {
	runes := []rune(norm.NFC.String(name))
	best := -1
	for _, alias := range memberAliases(m) {
		target := normalizedArtistName(alias)
		if target == "" {
			continue
		}
		for n := 1; n <= len(runes); n++ {
			prefix := normalizedArtistName(string(runes[:n]))
			if prefix != target {
				continue
			}
			if n < len(runes) && unicode.IsLetter(runes[n]) {
				continue
			}
			if n > best {
				best = n
			}
			break
		}
	}
	if best < 0 {
		return "", false
	}
	return string(runes[best:]), true
}

func catalogMembers(groups []artistGroup) []artistMember {
	result := []artistMember{}
	for _, g := range groups {
		result = append(result, g.Members...)
	}
	return result
}
func (s *Store) saveArtistMember(ctx context.Context, m artistMember) error {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" || len(m.Name) > 150 || len(m.Aliases) > 30 || len(m.Source) > 1000 {
		return fmt.Errorf("Invalid artist name, aliases or source")
	}
	for _, a := range m.Aliases {
		if strings.TrimSpace(a) == "" || len(a) > 200 {
			return fmt.Errorf("Invalid artist alias")
		}
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM artist_groups WHERE name=?", m.Group).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("Choose a group already present in the gallery")
	}
	aliases, _ := json.Marshal(m.Aliases)
	_, err := s.db.ExecContext(ctx, `INSERT INTO artist_members(group_name,name,aliases,source) VALUES(?,?,?,?) ON CONFLICT(group_name,name) DO UPDATE SET aliases=excluded.aliases,source=excluded.source`, m.Group, m.Name, string(aliases), m.Source)
	return err
}
