package gallery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This read-only adapter uses TeraBox's browser session API, not teraboxapi.com.
// Only trusted browser origins can receive account cookies.
type teraClient struct {
	baseURL string
	client  *http.Client
}

type teraSessionError struct{}

func (*teraSessionError) Error() string {
	return "TeraBox rejected this account session (errno -6). Match the account's API host to the browser /api/list request, then copy its complete Cookie and jsToken, save the session, and scan again."
}

func teraBrowserOrigin(host string) (string, error) {
	switch host {
	case "", "www.terabox.com":
		return "https://www.terabox.com", nil
	case "dm.terabox.com":
		return "https://dm.terabox.com", nil
	default:
		return "", errors.New("Choose www.terabox.com or dm.terabox.com as the TeraBox API host")
	}
}

type teraFile struct {
	ID       int64  `json:"fs_id"`
	Name     string `json:"server_filename"`
	Path     string `json:"path"`
	IsDir    int    `json:"isdir"`
	Created  int64  `json:"server_ctime"`
	Modified int64  `json:"server_mtime"`
}
type teraPage struct {
	Errno   *int       `json:"errno"`
	List    []teraFile `json:"list"`
	HasMore *bool      `json:"has_more"`
}

func newTeraClient() teraClient {
	return teraClient{baseURL: "https://www.terabox.com", client: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (t teraClient) list(ctx context.Context, session teraSession, dir string, pageNumber int) (teraPage, error) {
	var result teraPage
	origin := t.baseURL
	if session.Host != "" {
		var err error
		origin, err = teraBrowserOrigin(session.Host)
		if err != nil {
			return result, err
		}
	}
	q := url.Values{"app_id": {"250528"}, "web": {"1"}, "channel": {"dubox"}, "clienttype": {"0"}, "dir": {dir}, "order": {"time"}, "desc": {"1"}, "num": {"100"}, "page": {strconv.Itoa(pageNumber)}, "showempty": {"1"}}
	if session.BDSToken != "" {
		q.Set("bdstoken", session.BDSToken)
	}
	if session.JSToken != "" {
		q.Set("jsToken", session.JSToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/list?"+q.Encode(), nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Referer", origin+"/main")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	response, err := loggedDo(t.client, req)
	if err != nil {
		return result, errors.New("TeraBox connection failed; check network and reconnect the session")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return result, fmt.Errorf("TeraBox returned HTTP %d; reconnect the account if the session expired", response.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return result, errors.New("TeraBox response exceeded limit")
	}
	if json.Unmarshal(b, &result) != nil || result.Errno == nil {
		return result, errors.New("TeraBox did not return a file list; reconnect the account")
	}
	if *result.Errno == -6 {
		return result, &teraSessionError{}
	}
	if *result.Errno != 0 {
		return result, fmt.Errorf("TeraBox error %d; check the root path and reconnect the browser session", *result.Errno)
	}
	return result, nil
}

// Traverse old containers too: a recently added set can sit inside an old folder.
// A request budget bounds large drives; an incomplete scan is always surfaced.
func (t teraClient) scan(ctx context.Context, session teraSession, root string, budget int) ([]teraFile, bool, error) {
	result := []teraFile{}
	queue := []string{path.Clean(root)}
	seen := map[string]bool{}
	ids := map[int64]bool{}
	requests := 0
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if seen[dir] {
			continue
		}
		seen[dir] = true
		for pageNumber := 1; ; pageNumber++ {
			if requests >= budget {
				return result, false, nil
			}
			requests++
			page, err := t.list(ctx, session, dir, pageNumber)
			if err != nil {
				return nil, false, err
			}
			for _, f := range page.List {
				if f.ID <= 0 || f.Path == "" || len(f.Path) > 2000 || len(f.Name) > 500 || !strings.HasPrefix(f.Path, "/") || path.Dir(path.Clean(f.Path)) != dir {
					return nil, false, errors.New("TeraBox returned an invalid file identity or path")
				}
				if ids[f.ID] {
					continue
				}
				ids[f.ID] = true
				result = append(result, f)
				if f.IsDir == 1 {
					if _, _, err := splitTeraTitle(f.Name); err != nil {
						queue = append(queue, path.Clean(f.Path))
					}
				}
			}
			more := len(page.List) >= 100
			if page.HasMore != nil {
				more = *page.HasMore
			}
			if !more {
				break
			}
		}
	}
	return result, true, nil
}

var teraTitlePattern = regexp.MustCompile(`^([0-9]{6})\s+(.+)$`)

func splitTeraTitle(title string) (string, string, error) {
	parts := teraTitlePattern.FindStringSubmatch(strings.TrimSpace(title))
	if parts == nil {
		return "", "", errors.New("Folder name needs YYMMDD followed by the artist name")
	}
	date, ok := titleSetDate(parts[1])
	if !ok {
		return "", "", errors.New("Folder has an invalid photographed date")
	}
	return date.Format("2006-01-02"), strings.TrimSpace(parts[2]), nil
}
func recentTeraFolders(files []teraFile, now time.Time, days int) ([]teraFile, []string) {
	result := []teraFile{}
	warnings := []string{}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	for _, f := range files {
		if f.IsDir != 1 {
			continue
		}
		activity := f.activityTime()
		if activity == 0 {
			if len(warnings) < 100 {
				warnings = append(warnings, "Creation and modification times missing: "+f.Path)
			} else if len(warnings) == 100 {
				warnings = append(warnings, "Additional folders without timestamps were excluded; showing the first 100 warnings")
			}
			continue
		}
		if activity >= cutoff && activity <= now.Unix() {
			result = append(result, f)
		}
	}
	return result, warnings
}

func (f teraFile) activityTime() int64 {
	return max(f.Created, f.Modified)
}
