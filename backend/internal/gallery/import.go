package gallery

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

func ParseSheet(reader io.Reader, source string) ([]SetDraft, error) {
	parsed, err := parseSheetRows(reader, source)
	if err != nil {
		return nil, err
	}
	sets := make([]SetDraft, 0, len(parsed))
	for _, row := range parsed {
		sets = append(sets, row.Draft)
	}
	return sets, nil
}

type parsedSheetRow struct {
	Draft  SetDraft
	Number int // One-based row number in the source spreadsheet.
}

func parseSheetRows(reader io.Reader, source string) ([]parsedSheetRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid CSV: %w", err)
	}
	return parseRows(rows, source)
}

func parseRows(rows [][]string, source string) ([]parsedSheetRow, error) {
	header := -1
	columns := map[string]int{}
	for i, row := range rows {
		for j, c := range row {
			columns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, "\ufeff")))] = j
		}
		if _, ok := columns["name"]; ok {
			if _, ok := columns["group"]; ok {
				if _, ok := columns["date"]; ok {
					header = i
					break
				}
			}
		}
		columns = map[string]int{}
	}
	if header < 0 {
		return nil, fmt.Errorf("template needs Date, Name and GROUP columns")
	}
	sets := []parsedSheetRow{}
	occurrences := map[string]int{}
	for rowIndex, row := range rows[header+1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		cell := func(key string) string {
			i, ok := columns[key]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		raw, _ := json.Marshal(map[string]any{"header": rows[header], "cells": row})
		d := SetDraft{Title: cell("name"), Group: cell("group"), Source: cell("source"), Example: cell("example"), Notes: cell("remark"), RawDate: cell("date"), RawCells: string(raw)}
		if d.Title == "" {
			d.Title = "Untitled set"
		}
		if d.Group == "" {
			d.Group = "Unspecified"
		}
		if d.RawDate != "" && d.RawDate != "0" {
			parsed, e := time.Parse("060102", d.RawDate)
			if e != nil {
				parsed, e = time.Parse("2006-01-02", d.RawDate)
			}
			if e == nil && parsed.Year() >= 2000 {
				d.Date = parsed.Format("2006-01-02")
			} else {
				d.ImportWarning = "Unrecognized date: " + d.RawDate
			}
		}
		identity := source + "\x00" + d.Group + "\x00" + d.RawDate + "\x00" + d.Title
		if cell("id") != "" {
			identity = source + "\x00id:" + cell("id")
		}
		occurrences[identity]++
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", identity, occurrences[identity])))
		d.ImportKey = hex.EncodeToString(sum[:])
		sets = append(sets, parsedSheetRow{Draft: d, Number: header + rowIndex + 2})
	}
	if len(sets) == 0 {
		return nil, fmt.Errorf("template contains no data rows")
	}
	if len(sets) > 5000 {
		return nil, fmt.Errorf("template exceeds 5000 rows")
	}
	return sets, nil
}
