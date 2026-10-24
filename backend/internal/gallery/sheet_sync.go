package gallery

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrSheetConflict = errors.New("Google Sheet row changed; import the Sheet again before saving")
var ErrSheetUnavailable = errors.New("Google Sheet sync is unavailable")

// SheetSync writes only changed metadata cells. Google retains formatting and
// columns outside the imported template when values are updated in place.
type SheetSync struct {
	spreadsheetID string
	gid           int64
	source        string
	email         string
	privateKey    *rsa.PrivateKey
	client        *http.Client
	apiBase       string
	tokenURL      string
	writeMu       sync.Mutex
	tokenMu       sync.Mutex
	token         string
	tokenExpires  time.Time
}

func NewSheetSyncFromJSON(data []byte, sheetURL string) (*SheetSync, error) {
	_, source, err := SheetURL(sheetURL)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(source, ":")
	gid, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}
	var credentials struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err = json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("invalid Google Sheets credentials JSON: %w", err)
	}
	if credentials.Type != "service_account" || credentials.ClientEmail == "" || credentials.PrivateKey == "" || credentials.TokenURI != "https://oauth2.googleapis.com/token" {
		return nil, fmt.Errorf("Google Sheets credentials must be a service account JSON key")
	}
	block, _ := pem.Decode([]byte(credentials.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("Google Sheets private key is invalid")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Google Sheets private key is invalid: %w", err)
	}
	privateKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Google Sheets private key must be RSA")
	}
	return &SheetSync{spreadsheetID: parts[1], gid: gid, source: source, email: credentials.ClientEmail,
		privateKey: privateKey, client: &http.Client{Timeout: 20 * time.Second}, apiBase: "https://sheets.googleapis.com/v4", tokenURL: credentials.TokenURI}, nil
}

func (s *SheetSync) accessToken(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.token != "" && time.Until(s.tokenExpires) > time.Minute {
		return s.token, nil
	}
	now := time.Now()
	encode := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b), err
	}
	header, _ := encode(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, err := encode(map[string]any{"iss": s.email, "scope": "https://www.googleapis.com/auth/spreadsheets", "aud": s.tokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + claims
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("%w: request access token: %v", ErrSheetUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("%w: Google credential rejected (HTTP %d)", ErrSheetUnavailable, response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return "", fmt.Errorf("%w: invalid Google access token response", ErrSheetUnavailable)
	}
	s.token, s.tokenExpires = token.AccessToken, now.Add(time.Duration(token.ExpiresIn)*time.Second)
	return s.token, nil
}

func (s *SheetSync) request(ctx context.Context, method, endpoint string, body any, result any) error {
	token, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.apiBase+endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSheetUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: Google Sheets returned HTTP %d", ErrSheetUnavailable, response.StatusCode)
	}
	if result == nil {
		return nil
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(result); err != nil {
		return fmt.Errorf("%w: invalid Google Sheets response", ErrSheetUnavailable)
	}
	return nil
}

func (s *SheetSync) sheetTitle(ctx context.Context) (string, error) {
	var metadata struct {
		Sheets []struct {
			Properties struct {
				SheetID int64  `json:"sheetId"`
				Title   string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	endpoint := "/spreadsheets/" + s.spreadsheetID + "?fields=sheets.properties(sheetId,title)"
	if err := s.request(ctx, http.MethodGet, endpoint, nil, &metadata); err != nil {
		return "", err
	}
	for _, sheet := range metadata.Sheets {
		if sheet.Properties.SheetID == s.gid {
			return sheet.Properties.Title, nil
		}
	}
	return "", fmt.Errorf("%w: configured Sheet tab was not found", ErrSheetConflict)
}

func quoteSheetTitle(title string) string { return "'" + strings.ReplaceAll(title, "'", "''") + "'" }

func (s *SheetSync) rows(ctx context.Context, title string) ([][]string, error) {
	var values struct {
		Values [][]any `json:"values"`
	}
	endpoint := "/spreadsheets/" + s.spreadsheetID + "/values/" + url.PathEscape(quoteSheetTitle(title)) + "?valueRenderOption=FORMATTED_VALUE"
	if err := s.request(ctx, http.MethodGet, endpoint, nil, &values); err != nil {
		return nil, err
	}
	if len(values.Values) > 5000 {
		return nil, fmt.Errorf("%w: Sheet exceeds 5000 rows", ErrSheetConflict)
	}
	rows := make([][]string, len(values.Values))
	for i, row := range values.Values {
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			rows[i][j] = fmt.Sprint(cell)
		}
	}
	return rows, nil
}

func parseLiveRows(rows [][]string, source string) ([]parsedSheetRow, error) {
	// Keep blank entries so Number remains the physical Google Sheet row.
	return parseRows(rows, source)
}

func rawSheetCells(raw string) (header, cells []string, err error) {
	var record struct {
		Header []string `json:"header"`
		Cells  []string `json:"cells"`
	}
	err = json.Unmarshal([]byte(raw), &record)
	return record.Header, record.Cells, err
}

func sheetColumn(header []string, name string) int {
	for i, cell := range header {
		if strings.EqualFold(strings.TrimSpace(cell), name) {
			return i
		}
	}
	return -1
}

func columnLetter(index int) string {
	result := ""
	for index >= 0 {
		result = string(rune('A'+index%26)) + result
		index = index/26 - 1
	}
	return result
}

func sheetDate(date, oldRaw string) string {
	if date == "" {
		if oldRaw == "0" {
			return "0"
		}
		return ""
	}
	if len(oldRaw) == 6 || oldRaw == "0" {
		parsed, _ := time.Parse("2006-01-02", date)
		return parsed.Format("060102")
	}
	return date
}

// InsertNew adds a row within the existing group ordered by the date in Name.
// The Sheet Date column records the Bangkok date when the set was added.
func (s *SheetSync) InsertNew(ctx context.Context, store *Store, requested SetDraft, setDate time.Time) (SetDraft, int, error) {
	return s.insertNew(ctx, store, requested, setDate, false)
}

// TeraBox retries keep a title unique across added dates, including after a
// failed request is recovered on a later day. Manual insertion retains its
// existing title-plus-added-date identity rule.
func (s *SheetSync) insertNew(ctx context.Context, store *Store, requested SetDraft, setDate time.Time, uniqueTitle bool) (SetDraft, int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	title, err := s.sheetTitle(ctx)
	if err != nil {
		return requested, 0, err
	}
	rows, err := s.rows(ctx, title)
	if err != nil {
		return requested, 0, err
	}
	parsed, err := parseLiveRows(rows, s.source)
	if err != nil {
		return requested, 0, fmt.Errorf("%w: %v", ErrSheetConflict, err)
	}
	header, _, err := rawSheetCells(parsed[0].Draft.RawCells)
	if err != nil {
		return requested, 0, fmt.Errorf("%w: invalid Sheet header", ErrSheetConflict)
	}
	for _, row := range parsed {
		if strings.EqualFold(strings.TrimSpace(row.Draft.Group), strings.TrimSpace(requested.Group)) {
			if row.Draft.Title == requested.Title && (uniqueTitle || row.Draft.Date == requested.Date) {
				return requested, 0, fmt.Errorf("%w: this set already appears in the Sheet; import it before retrying", ErrSheetConflict)
			}
			requested.Group = row.Draft.Group
		}
	}
	values := map[string]string{
		"date": setDateAdded(requested.Date), "name": requested.Title,
		"group": requested.Group, "source": requested.Source,
		"example": requested.Example, "remark": requested.Notes,
	}
	newRow := make([]string, len(header))
	for name, value := range values {
		column := sheetColumn(header, name)
		if column < 0 {
			return requested, 0, fmt.Errorf("%w: %s column is missing", ErrSheetConflict, name)
		}
		newRow[column] = value
	}
	insertIndex := len(rows)
	lastGroupIndex := -1
	for _, row := range parsed {
		if !strings.EqualFold(strings.TrimSpace(row.Draft.Group), strings.TrimSpace(requested.Group)) {
			continue
		}
		lastGroupIndex = row.Number - 1
		if existingDate, ok := titleSetDate(row.Draft.Title); ok && existingDate.Before(setDate) {
			insertIndex = row.Number - 1
			break
		}
	}
	if insertIndex == len(rows) && lastGroupIndex >= 0 {
		insertIndex = lastGroupIndex + 1
	}
	updatedRows := make([][]string, 0, len(rows)+1)
	updatedRows = append(updatedRows, rows[:insertIndex]...)
	updatedRows = append(updatedRows, newRow)
	updatedRows = append(updatedRows, rows[insertIndex:]...)
	updated, err := parseLiveRows(updatedRows, s.source)
	if err != nil {
		return requested, 0, fmt.Errorf("%w: inserted row is invalid: %v", ErrSheetConflict, err)
	}
	var inserted SetDraft
	oldIndex := 0
	for _, row := range updated {
		if row.Number == insertIndex+1 {
			inserted = row.Draft
			continue
		}
		if oldIndex >= len(parsed) || row.Draft.ImportKey != parsed[oldIndex].Draft.ImportKey {
			return requested, 0, fmt.Errorf("%w: insertion would change another row's identity; add an ID column first", ErrSheetConflict)
		}
		oldIndex++
	}
	if inserted.ImportKey == "" || oldIndex != len(parsed) {
		return requested, 0, fmt.Errorf("%w: inserted row identity is invalid", ErrSheetConflict)
	}
	if err := store.CheckImportKey(ctx, 0, inserted.ImportKey); err != nil {
		return requested, 0, fmt.Errorf("%w: %v", ErrSheetConflict, err)
	}
	// Copy the nearest data row's format and validation, then set only values.
	formatSource := insertIndex - 1
	if insertIndex < len(rows) && (lastGroupIndex < 0 || insertIndex <= lastGroupIndex) {
		formatSource = insertIndex + 1 // Row after insertion has shifted down.
	}
	cellValues := make([]map[string]any, len(newRow))
	for i, value := range newRow {
		cellValues[i] = map[string]any{"userEnteredValue": map[string]string{"stringValue": value}}
	}
	grid := func(rowIndex int) map[string]any {
		return map[string]any{"sheetId": s.gid, "startRowIndex": rowIndex, "endRowIndex": rowIndex + 1, "startColumnIndex": 0, "endColumnIndex": len(header)}
	}
	requests := []map[string]any{
		{"insertDimension": map[string]any{"range": map[string]any{"sheetId": s.gid, "dimension": "ROWS", "startIndex": insertIndex, "endIndex": insertIndex + 1}, "inheritFromBefore": insertIndex > 0}},
		{"copyPaste": map[string]any{"source": grid(formatSource), "destination": grid(insertIndex), "pasteType": "PASTE_FORMAT"}},
		{"copyPaste": map[string]any{"source": grid(formatSource), "destination": grid(insertIndex), "pasteType": "PASTE_DATA_VALIDATION"}},
		{"updateCells": map[string]any{"start": map[string]any{"sheetId": s.gid, "rowIndex": insertIndex, "columnIndex": 0}, "rows": []any{map[string]any{"values": cellValues}}, "fields": "userEnteredValue"}},
	}
	endpoint := "/spreadsheets/" + s.spreadsheetID + ":batchUpdate"
	if err := s.request(ctx, http.MethodPost, endpoint, map[string]any{"requests": requests}, nil); err != nil {
		return requested, 0, err
	}
	requested.RawDate, requested.RawCells, requested.ImportKey, requested.ImportWarning = inserted.RawDate, inserted.RawCells, inserted.ImportKey, inserted.ImportWarning
	return requested, insertIndex + 1, nil
}

func setDateAdded(date string) string {
	parsed, _ := time.Parse("2006-01-02", date)
	return parsed.Format("060102")
}

// UpdateExisting verifies the imported row still exists and that the cells
// being edited have not changed since import. It returns import metadata for
// the local save, so the next import still matches a renamed row.
func (s *SheetSync) UpdateExisting(ctx context.Context, store *Store, old Set, requested SetDraft, oldKey string) (SetDraft, bool, error) {
	changes := map[string]string{}
	if old.Title != requested.Title {
		changes["name"] = requested.Title
	}
	if old.Group != requested.Group {
		changes["group"] = requested.Group
	}
	if old.Date != requested.Date {
		changes["date"] = sheetDate(requested.Date, old.RawDate)
	}
	if old.Source != requested.Source {
		changes["source"] = requested.Source
	}
	if old.Example != requested.Example {
		changes["example"] = requested.Example
	}
	if old.Notes != requested.Notes {
		changes["remark"] = requested.Notes
	}
	if len(changes) == 0 {
		return requested, false, nil
	}
	if oldKey == "" {
		return requested, false, fmt.Errorf("%w: set has no Sheet identity", ErrSheetConflict)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	title, err := s.sheetTitle(ctx)
	if err != nil {
		return requested, false, err
	}
	rows, err := s.rows(ctx, title)
	if err != nil {
		return requested, false, err
	}
	parsed, err := parseLiveRows(rows, s.source)
	if err != nil {
		return requested, false, fmt.Errorf("%w: %v", ErrSheetConflict, err)
	}
	var matched *parsedSheetRow
	for i := range parsed {
		if parsed[i].Draft.ImportKey == oldKey {
			matched = &parsed[i]
			break
		}
	}
	if matched == nil {
		return requested, false, fmt.Errorf("%w: matching row was not found", ErrSheetConflict)
	}
	oldHeader, oldCells, err := rawSheetCells(old.RawCells)
	if err != nil {
		return requested, false, fmt.Errorf("%w: stored row snapshot is invalid", ErrSheetConflict)
	}
	liveHeader, _, err := rawSheetCells(matched.Draft.RawCells)
	if err != nil {
		return requested, false, err
	}
	for _, name := range []string{"date", "name", "group", "source", "example", "remark"} {
		oldCol, col := sheetColumn(oldHeader, name), sheetColumn(liveHeader, name)
		if oldCol < 0 || col < 0 {
			if oldCol != col {
				return requested, false, fmt.Errorf("%w: %s column changed", ErrSheetConflict, name)
			}
			continue
		}
		previous, current := "", ""
		if oldCol < len(oldCells) {
			previous = strings.TrimSpace(oldCells[oldCol])
		}
		if col < len(rows[matched.Number-1]) {
			current = strings.TrimSpace(rows[matched.Number-1][col])
		}
		if previous != current {
			return requested, false, fmt.Errorf("%w: %s was edited in the Sheet", ErrSheetConflict, name)
		}
	}
	updatedRows := make([][]string, len(rows))
	copy(updatedRows, rows)
	rowIndex := matched.Number - 1
	updatedRows[rowIndex] = append([]string(nil), rows[rowIndex]...)
	data := []map[string]any{}
	for name, value := range changes {
		col := sheetColumn(liveHeader, name)
		if col < 0 {
			return requested, false, fmt.Errorf("%w: %s column is missing", ErrSheetConflict, name)
		}
		for len(updatedRows[rowIndex]) <= col {
			updatedRows[rowIndex] = append(updatedRows[rowIndex], "")
		}
		updatedRows[rowIndex][col] = value
		cell := quoteSheetTitle(title) + "!" + columnLetter(col) + strconv.Itoa(matched.Number)
		data = append(data, map[string]any{"range": cell, "values": [][]string{{value}}})
	}
	updated, err := parseLiveRows(updatedRows, s.source)
	if err != nil {
		return requested, false, err
	}
	for i := range parsed {
		if parsed[i].Number != matched.Number && parsed[i].Draft.ImportKey != updated[i].Draft.ImportKey {
			return requested, false, fmt.Errorf("%w: this edit would change another row's identity; add an ID column before renaming", ErrSheetConflict)
		}
	}
	var synced SetDraft
	for _, row := range updated {
		if row.Number == matched.Number {
			synced = row.Draft
			break
		}
	}
	if synced.ImportKey == "" {
		return requested, false, fmt.Errorf("%w: updated row is invalid", ErrSheetConflict)
	}
	if err := store.CheckImportKey(ctx, old.ID, synced.ImportKey); err != nil {
		return requested, false, fmt.Errorf("%w: %v", ErrSheetConflict, err)
	}
	endpoint := "/spreadsheets/" + s.spreadsheetID + "/values:batchUpdate"
	if err := s.request(ctx, http.MethodPost, endpoint, map[string]any{"valueInputOption": "RAW", "data": data}, nil); err != nil {
		return requested, false, err
	}
	requested.RawDate, requested.RawCells, requested.ImportKey, requested.ImportWarning = synced.RawDate, synced.RawCells, synced.ImportKey, synced.ImportWarning
	return requested, true, nil
}
