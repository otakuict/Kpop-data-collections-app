package gallery

import (
	"context"
	"database/sql"
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

const GankInterval = 20 * time.Second
const PageCacheTTL = 7 * 24 * time.Hour

type RemotePausedError struct {
	Host   string
	Until  time.Time
	Reason string
}

func (e *RemotePausedError) Error() string {
	return fmt.Sprintf("%s ingestion paused until %s (%s)", e.Host, e.Until.UTC().Format(time.RFC3339), e.Reason)
}
func remoteKey(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "ganknow.com" || strings.HasSuffix(host, ".ganknow.com") {
		return "ganknow.com"
	}
	return host
}
func (s *Store) reserveRemote(ctx context.Context, host string, now time.Time, interval time.Duration) (time.Duration, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO remote_limits(host) VALUES(?)", host); err != nil {
		return 0, err
	}
	var next, paused int64
	var reason string
	if err = tx.QueryRowContext(ctx, "SELECT next_at,paused_until,reason FROM remote_limits WHERE host=?", host).Scan(&next, &paused, &reason); err != nil {
		return 0, err
	}
	if paused > now.UnixMilli() {
		return 0, &RemotePausedError{host, time.UnixMilli(paused), reason}
	}
	if next > now.UnixMilli() {
		return time.UnixMilli(next).Sub(now), nil
	}
	if host == "ganknow.com" {
		var leaseUntil int64
		err = tx.QueryRowContext(ctx, "SELECT until FROM remote_leases WHERE host=?", host).Scan(&leaseUntil)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		if leaseUntil > now.UnixMilli() {
			return time.UnixMilli(leaseUntil).Sub(now), nil
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO remote_leases(host,until) VALUES(?,?) ON CONFLICT(host) DO UPDATE SET until=excluded.until", host, now.Add(95*time.Second).UnixMilli()); err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE remote_limits SET next_at=? WHERE host=?", now.Add(interval).UnixMilli(), host); err != nil {
		return 0, err
	}
	return 0, tx.Commit()
}
func (s *Store) pauseRemote(ctx context.Context, host string, until time.Time, reason string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO remote_limits(host,paused_until,reason) VALUES(?,?,?) ON CONFLICT(host) DO UPDATE SET paused_until=MAX(paused_until,excluded.paused_until),reason=excluded.reason`, host, until.UnixMilli(), reason)
	return err
}
func (s *Store) waitRemote(ctx context.Context, host string) error {
	interval := time.Second
	if host == "ganknow.com" {
		interval = GankInterval
	}
	for {
		wait, err := s.reserveRemote(ctx, host, time.Now(), interval)
		if err != nil || wait == 0 {
			return err
		}
		timer := time.NewTimer(min(wait, time.Second))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func retryTime(header string, now time.Time, fallback time.Duration) time.Time {
	until := now.Add(fallback)
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds > 0 && seconds <= 31536000 {
		if candidate := now.Add(time.Duration(seconds) * time.Second); candidate.After(until) {
			return candidate
		}
	}
	if candidate, err := http.ParseTime(header); err == nil && candidate.After(until) {
		return candidate
	}
	return until
}

func (s *Store) releaseRemote(ctx context.Context, host string, now time.Time) error {
	if host != "ganknow.com" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM remote_leases WHERE host=?", host); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE remote_limits SET next_at=MAX(next_at,?) WHERE host=?", now.Add(GankInterval).UnixMilli(), host); err != nil {
		return err
	}
	return tx.Commit()
}

type remoteBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

type sourceStatusError struct{ Code int }

func (e *sourceStatusError) Error() string { return fmt.Sprintf("source returned HTTP %d", e.Code) }

func (b *remoteBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }

type remoteTransport struct {
	base  http.RoundTripper
	store *Store
}

func (t *remoteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, err := ParseRemoteURL(req.URL.String()); err != nil {
		return nil, err
	}
	host := remoteKey(req.URL.Hostname())
	if err := t.store.waitRemote(req.Context(), host); err != nil {
		return nil, err
	}
	release := func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
		defer cancel()
		t.store.releaseRemote(ctx, host, time.Now())
	}
	response, err := t.base.RoundTrip(req)
	if err != nil {
		release()
		return nil, err
	}
	response.Body = &remoteBody{ReadCloser: response.Body, release: release}
	if response.StatusCode != 429 && response.StatusCode != 403 && response.Header.Get("Cf-Mitigated") != "challenge" {
		return response, nil
	}
	fallback := time.Hour
	if response.StatusCode == 403 || response.Header.Get("Cf-Mitigated") == "challenge" {
		fallback = 24 * time.Hour
	}
	until := retryTime(response.Header.Get("Retry-After"), time.Now(), fallback)
	reason := fmt.Sprintf("source HTTP %d", response.StatusCode)
	response.Body.Close()
	// Record the pause even if the caller disconnected after the remote response.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
	defer cancel()
	if err := t.store.pauseRemote(saveCtx, host, until, reason); err != nil {
		return nil, err
	}
	return nil, &RemotePausedError{host, until, reason}
}
func imageRequestURL(raw string) (*url.URL, error) {
	u, err := ParseRemoteURL(raw)
	if err != nil {
		return nil, err
	}
	// Unsized Google image URLs can resolve to a small preview. Request the
	// same 1024-pixel variant shown on the source page instead.
	if u.Hostname() == "lh3.googleusercontent.com" && !googleImageSuffix.MatchString(u.Path) {
		u.Path += "=s1024"
		u.RawPath = ""
	}
	return u, nil
}
func fetchStored(ctx context.Context, store *Store, raw string, max int64) ([]byte, string, error) {
	u, err := imageRequestURL(raw)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ensureRequestID(ctx), "GET", u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "BiasArchive/1.0 public-preview-ingestion")
	req.Header.Set("Accept", "text/html,image/*")
	client := *fetchClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = &loggingTransport{base: &remoteTransport{base: base, store: store}}
	// Timeout includes the shared schedule wait and all redirects.
	client.Timeout = 90 * time.Second
	response, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", &sourceStatusError{Code: response.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > max {
		return nil, "", fmt.Errorf("source exceeds size limit")
	}
	return b, response.Header.Get("Content-Type"), nil
}
func fetchDocument(ctx context.Context, store *Store, raw string) ([]byte, string, error) {
	var b []byte
	var mime string
	err := store.db.QueryRowContext(ctx, "SELECT bytes,mime FROM page_cache WHERE url=? AND fetched_at>?", raw, time.Now().Add(-PageCacheTTL).UnixMilli()).Scan(&b, &mime)
	if err == nil {
		return b, mime, nil
	}
	if err != sql.ErrNoRows {
		return nil, "", err
	}
	b, mime, err = fetchStored(ctx, store, raw, MaxImageBytes)
	if err != nil {
		return nil, "", err
	}
	if !strings.HasPrefix(mime, "image/") {
		err = store.CachePage(ctx, raw, b, mime)
	}
	return b, mime, err
}
func (s *Store) CachePage(ctx context.Context, raw string, b []byte, mime string) error {
	if _, err := ParseRemoteURL(raw); err != nil {
		return err
	}
	if len(b) > MaxImageBytes {
		return fmt.Errorf("page exceeds size limit")
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO page_cache(url,bytes,mime,fetched_at) VALUES(?,?,?,?) ON CONFLICT(url) DO UPDATE SET bytes=excluded.bytes,mime=excluded.mime,fetched_at=excluded.fetched_at", raw, b, mime, time.Now().UnixMilli())
	return err
}

type IngestionStatus struct {
	IntervalSeconds int    `json:"intervalSeconds"`
	MinImages       int    `json:"minImages"`
	MaxImages       int    `json:"maxImages"`
	PausedUntil     string `json:"pausedUntil"`
	Reason          string `json:"reason"`
}

func (s *Store) IngestionStatus(ctx context.Context) (IngestionStatus, error) {
	result := IngestionStatus{IntervalSeconds: int(GankInterval / time.Second), MinImages: MinSetImages, MaxImages: MaxSetImages}
	var until int64
	err := s.db.QueryRowContext(ctx, "SELECT paused_until,reason FROM remote_limits WHERE host='ganknow.com'").Scan(&until, &result.Reason)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if until > time.Now().UnixMilli() {
		result.PausedUntil = time.UnixMilli(until).UTC().Format(time.RFC3339)
	} else {
		result.Reason = ""
	}
	return result, nil
}
func isRemotePaused(err error) bool { var pause *RemotePausedError; return errors.As(err, &pause) }
