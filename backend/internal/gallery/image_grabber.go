package gallery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxGrabberResponseBytes = 2 << 20

func grabberConfigured() bool { return strings.TrimSpace(os.Getenv("GANK_IMAGE_GRABBER_URL")) != "" }

func fetchGankGrabberImages(ctx context.Context, id SetID, example string) (urls []string, resultErr error) {
	ctx = ensureRequestID(ctx)
	started := time.Now()
	defer func() { processFinished(ctx, "image_grabber", id, 0, example, started, resultErr) }()
	base, err := url.Parse(strings.TrimSpace(os.Getenv("GANK_IMAGE_GRABBER_URL")))
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("GANK_IMAGE_GRABBER_URL must be an HTTP(S) API root without credentials or query parameters")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/extract"
	base.RawPath = ""
	base.RawQuery = url.Values{"url": {example}}.Encode()
	req, err := http.NewRequestWithContext(ensureRequestID(ctx), http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// Only the operator-configured service may use internal HTTP. Image downloads
	// still go through the public HTTPS/DNS checks in fetchStored.
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := loggedDo(client, req)
	if err != nil {
		return nil, fmt.Errorf("image grabber unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image grabber returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGrabberResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxGrabberResponseBytes {
		return nil, fmt.Errorf("image grabber response exceeds size limit")
	}
	var result struct {
		SourceURL string `json:"sourceUrl"`
		Images    []struct {
			Type        string `json:"type"`
			RenderedURL string `json:"renderedUrl"`
			OriginalURL string `json:"originalUrl"`
			ThumbURL    string `json:"thumbUrl"`
		} `json:"images"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid image grabber JSON: %w", err)
	}
	wanted, ok := exampleGankURL(example)
	source, sourceOK := exampleGankURL(result.SourceURL)
	if !ok || !sourceOK || source != wanted {
		return nil, fmt.Errorf("image grabber returned a different source page")
	}
	seen := map[string]bool{}
	for _, img := range result.Images {
		if img.Type != "image" {
			continue
		}
		for _, raw := range []string{img.RenderedURL, img.OriginalURL, img.ThumbURL} {
			selected := grabberImageURL(raw)
			if selected == "" {
				continue
			}
			if !seen[selected] {
				seen[selected] = true
				urls = append(urls, selected)
			}
			break
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("image grabber returned no supported public images")
	}
	return urls, nil
}

func grabberImageURL(raw string) string {
	u, err := ParseRemoteURL(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	switch {
	case host == "lh3.googleusercontent.com":
		// Match gallery storage resolution without downloading the full-size original.
		u.Path = googleImageSuffix.ReplaceAllString(u.Path, "") + "=s1200"
		u.RawPath = ""
	case host == "media.ganknow.com":
	case strings.HasSuffix(host, ".r2.cloudflarestorage.com"):
		// Keep signed R2 URLs byte-for-byte, including their query and escaped path.
		return raw
	default:
		return ""
	}
	return u.String()
}

// Resizing or renewing a signature does not make the source a different photo.
func imageSourceKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Hostname() == "lh3.googleusercontent.com" {
		u.Path = googleImageSuffix.ReplaceAllString(u.Path, "")
		u.RawPath = ""
	}
	if strings.HasSuffix(u.Hostname(), ".r2.cloudflarestorage.com") {
		query := u.Query()
		for key := range query {
			if strings.HasPrefix(strings.ToLower(key), "x-amz-") {
				query.Del(key)
			}
		}
		u.RawQuery = query.Encode()
	}
	return u.String()
}
