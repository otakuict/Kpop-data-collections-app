package gallery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var gankServiceSlugPattern = regexp.MustCompile(`^[0-9]+-[A-Za-z0-9-]{1,200}$`)

func gankServiceSlug(raw string) (string, bool) {
	u, err := ParseRemoteURL(raw)
	if err != nil {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host != "ganknow.com" && host != "www.ganknow.com" {
		return "", false
	}
	if !strings.HasPrefix(u.Path, "/services/") {
		return "", false
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/services/"), "/")
	return slug, gankServiceSlugPattern.MatchString(slug)
}

// Gank's service page loads its gallery from the listing API after the initial HTML response.
func fetchGankServiceImages(ctx context.Context, store *Store, slug string) ([]string, error) {
	base := "https://api.ganknow.com/v1/catalogs/services/slug/" + url.PathEscape(slug)
	b, _, err := fetchDocument(ctx, store, base)
	if err != nil {
		var status *sourceStatusError
		if !errors.As(err, &status) || status.Code != 404 {
			return nil, err
		}
		b, _, err = fetchDocument(ctx, store, base+"/old")
		if err != nil {
			return nil, err
		}
	}
	urls, err := gankServiceImages(b)
	if err != nil {
		return nil, fmt.Errorf("invalid Gank service data: %w", err)
	}
	return urls, nil
}

func gankServiceImages(b []byte) ([]string, error) {
	var response struct {
		Data struct {
			Media []struct {
				Type   string `json:"type"`
				Active *bool  `json:"isActive"`
				URL    string `json:"url"`
			} `json:"media"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	urls := []string{}
	seen := map[string]bool{}
	for _, media := range response.Data.Media {
		if media.Type != "image" || media.Active != nil && !*media.Active {
			continue
		}
		u, err := ParseRemoteURL(media.URL)
		if err != nil {
			continue
		}
		host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
		if host != "lh3.googleusercontent.com" && (host != "media.ganknow.com" || !strings.HasPrefix(u.Path, "/public/")) {
			continue
		}
		if !seen[u.String()] {
			seen[u.String()] = true
			urls = append(urls, u.String())
		}
	}
	return urls, nil
}

var nuxtCall = regexp.MustCompile(`(?s)window\.__NUXT__\s*=\s*\(function\(([^)]*)\)\s*\{.*\}\s*\((.*)\)\s*\);?\s*$`)
var nuxtPost = regexp.MustCompile(`(?s)\bpost\s*:\s*\{\s*id\s*:\s*([^,{}]+),(.*?)\bpostMedia\s*:\s*\[([^\]]*)\]`)
var nuxtMedia = regexp.MustCompile(`\{([^{}]*)\}`)
var nuxtURL = regexp.MustCompile(`(?:^|,)\s*url\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)
var nuxtType = regexp.MustCompile(`(?:^|,)\s*type\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)
var nuxtAccess = regexp.MustCompile(`(?:^|,)\s*accessType\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)

// Read only JSON literal arguments and image references; never execute the Nuxt script.
func gankNuxtPostImages(script, postID string) []string {
	call := nuxtCall.FindStringSubmatch(script)
	if len(call) != 3 {
		return nil
	}
	params := strings.Split(call[1], ",")
	var args []json.RawMessage
	if json.Unmarshal([]byte("["+call[2]+"]"), &args) != nil || len(params) != len(args) {
		return nil
	}
	aliases := map[string]string{}
	for i, param := range params {
		var value string
		if json.Unmarshal(args[i], &value) == nil {
			aliases[strings.TrimSpace(param)] = value
		}
	}
	resolve := func(raw string) string {
		var value string
		if json.Unmarshal([]byte(strings.TrimSpace(raw)), &value) == nil {
			return value
		}
		return aliases[strings.TrimSpace(raw)]
	}
	urls := []string{}
	for _, post := range nuxtPost.FindAllStringSubmatch(script, -1) {
		access := nuxtAccess.FindStringSubmatch(post[2])
		if resolve(post[1]) != postID || len(access) != 2 || resolve(access[1]) != "public" {
			continue
		}
		for _, media := range nuxtMedia.FindAllStringSubmatch(post[3], -1) {
			url := nuxtURL.FindStringSubmatch(media[1])
			typeRef := nuxtType.FindStringSubmatch(media[1])
			if len(url) == 2 && len(typeRef) == 2 && resolve(typeRef[1]) == "image" {
				urls = append(urls, resolve(url[1]))
			}
		}
	}
	return urls
}
