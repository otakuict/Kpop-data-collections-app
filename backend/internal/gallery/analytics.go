package gallery

import (
	"context"
	"sort"
	"time"
)

type UsageTotals struct {
	Visitors       int     `json:"visitors"`
	Visits         int     `json:"visits"`
	TotalSeconds   int64   `json:"totalSeconds"`
	AverageSeconds float64 `json:"averageSeconds"`
}

type DailyUsage struct {
	Date string `json:"date"`
	UsageTotals
}

type CountryUsage struct {
	Country string `json:"country"`
	UsageTotals
}

type UsageReport struct {
	UsageTotals
	Online    int            `json:"online"`
	Days      int            `json:"days"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Daily     []DailyUsage   `json:"daily"`
	Countries []CountryUsage `json:"countries"`
}

type usageBucket struct {
	UsageTotals
	visitors map[string]struct{}
}

func (s *Store) pruneUsage(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM usage_visits WHERE last_seen<?`, now.Add(-90*24*time.Hour).Unix())
	return err
}

func (b *usageBucket) add(visitor string, seconds int64) {
	if b.visitors == nil {
		b.visitors = make(map[string]struct{})
	}
	b.visitors[visitor] = struct{}{}
	b.Visitors = len(b.visitors)
	b.Visits++
	b.TotalSeconds += seconds
	b.AverageSeconds = float64(b.TotalSeconds) / float64(b.Visits)
}

func (s *Store) onlineVisitors(ctx context.Context, now time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT visitor_id) FROM usage_visits WHERE last_seen>? AND last_seen<=?`, now.Add(-presenceTTL).Unix(), now.Unix()).Scan(&count)
	return count, err
}

func (s *Store) Usage(ctx context.Context, now time.Time, days int) (UsageReport, error) {
	if err := s.pruneUsage(ctx, now); err != nil {
		return UsageReport{}, err
	}
	today := now.UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, 1-days)
	report := UsageReport{Days: days, From: from.Format("2006-01-02"), To: today.Format("2006-01-02"), Daily: make([]DailyUsage, days), Countries: []CountryUsage{}}
	online, err := s.onlineVisitors(ctx, now)
	if err != nil {
		return report, err
	}
	report.Online = online
	rows, err := s.db.QueryContext(ctx, `SELECT visitor_id,country,MAX(started_at,?),MIN(last_seen,?) FROM usage_visits WHERE last_seen>=? AND started_at<=?`, from.Unix(), now.Unix(), from.Unix(), now.Unix())
	if err != nil {
		return report, err
	}
	defer rows.Close()
	var total usageBucket
	daily := make([]usageBucket, days)
	countries := make(map[string]*usageBucket)
	for rows.Next() {
		var visitor, country string
		var start, end int64
		if err := rows.Scan(&visitor, &country, &start, &end); err != nil {
			return report, err
		}
		total.add(visitor, end-start)
		if countries[country] == nil {
			countries[country] = &usageBucket{}
		}
		countries[country].add(visitor, end-start)
		firstDay := int((start - from.Unix()) / 86400)
		lastDay := min(days-1, int((end-from.Unix())/86400))
		for day := firstDay; day <= lastDay; day++ {
			dayStart := from.Unix() + int64(day)*86400
			daily[day].add(visitor, min(end, dayStart+86400)-max(start, dayStart))
		}
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	report.UsageTotals = total.UsageTotals
	for day := range daily {
		report.Daily[day] = DailyUsage{Date: from.AddDate(0, 0, day).Format("2006-01-02"), UsageTotals: daily[day].UsageTotals}
	}
	for country, bucket := range countries {
		report.Countries = append(report.Countries, CountryUsage{Country: country, UsageTotals: bucket.UsageTotals})
	}
	sort.Slice(report.Countries, func(i, j int) bool {
		if report.Countries[i].Visitors == report.Countries[j].Visitors {
			if report.Countries[i].Country == "" {
				return false
			}
			if report.Countries[j].Country == "" {
				return true
			}
			return report.Countries[i].Country < report.Countries[j].Country
		}
		return report.Countries[i].Visitors > report.Countries[j].Visitors
	})
	return report, nil
}
