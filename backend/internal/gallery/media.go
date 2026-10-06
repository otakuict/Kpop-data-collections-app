package gallery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/net/html"
)

const MaxImageBytes = 12 << 20

type Raster struct {
	expectedExample *string
	Bytes           []byte
	MIME, SourceURL string
}

func publicAddress(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96"} {
		if netip.MustParsePrefix(raw).Contains(addr) {
			return false
		}
	}
	return true
}
func ParseRemoteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return nil, fmt.Errorf("only public HTTPS URLs on port 443 are supported")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicAddress(ip) {
		return nil, fmt.Errorf("private or reserved address refused")
	}
	return u, nil
}

func remoteClient() *http.Client {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, MaxIdleConns: 20, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if port != "443" {
			return nil, fmt.Errorf("port refused")
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no public DNS address")
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, fmt.Errorf("private or reserved DNS address refused")
			}
		}
		dialer := net.Dialer{Timeout: 10 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		_, err := ParseRemoteURL(req.URL.String())
		return err
	}}
}

var fetchClient = remoteClient()
var fetchSchedule = struct {
	sync.Mutex
	next map[string]time.Time
}{next: map[string]time.Time{}}

func waitForHost(ctx context.Context, host string) error {
	fetchSchedule.Lock()
	start := fetchSchedule.next[host]
	if start.Before(time.Now()) {
		start = time.Now()
	}
	fetchSchedule.next[host] = start.Add(time.Second)
	fetchSchedule.Unlock()
	timer := time.NewTimer(time.Until(start))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Fetch(ctx context.Context, raw string, max int64) ([]byte, string, error) {
	u, err := ParseRemoteURL(raw)
	if err != nil {
		return nil, "", err
	}
	if err := waitForHost(ctx, u.Hostname()); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "BiasArchive/1.0 public-preview-ingestion")
	req.Header.Set("Accept", "text/html,image/*,text/csv")
	response, err := fetchClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, "", fmt.Errorf("source returned HTTP %d", response.StatusCode)
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

var cropSuffix = regexp.MustCompile(`=w\d+(?:-h\d+)?(?:-[a-z]+)?$`)

var scriptURL = regexp.MustCompile(`"https:(?:\\.|[^"\\])*"`)

func ExtractImages(document, base string) []string {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil
	}
	gank := remoteKey(baseURL.Hostname()) == "ganknow.com"
	gumroad := baseURL.Hostname() == "gumroad.com" || strings.HasSuffix(baseURL.Hostname(), ".gumroad.com")
	urls := []string{}
	seen := map[string]bool{}
	add := func(raw string, postMedia bool) {
		u, e := url.Parse(raw)
		if e != nil {
			return
		}
		u = baseURL.ResolveReference(u)
		if gank {
			if u.Hostname() != "media.ganknow.com" || (postMedia && !strings.Contains(u.Path, "/PM.")) {
				return
			}
			raw = cropSuffix.ReplaceAllString(u.String(), "")
		} else {
			raw = u.String()
		}
		if _, e = ParseRemoteURL(raw); e == nil && !seen[raw] {
			seen[raw] = true
			urls = append(urls, raw)
		}
	}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			key := attrs["property"]
			if key == "" {
				key = attrs["name"]
			}
			if key == "og:image" || key == "og:image:secure_url" || key == "twitter:image" {
				add(attrs["content"], false)
			}
		}
		if gumroad && n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key != "data-page" {
					continue
				}
				var page struct {
					Props struct {
						Product struct {
							Covers []struct {
								URL  string `json:"url"`
								Type string `json:"type"`
							} `json:"covers"`
						} `json:"product"`
					} `json:"props"`
				}
				if json.Unmarshal([]byte(attr.Val), &page) != nil {
					continue
				}
				for _, cover := range page.Props.Product.Covers {
					if cover.Type == "image" {
						add(cover.URL, false)
					}
				}
			}
		}
		if gank && n.Type == html.ElementNode && n.Data == "script" && n.FirstChild != nil {
			script := n.FirstChild.Data
			if strings.Contains(script, "window.__NUXT__") && strings.Contains(script, "postMedia") {
				for _, literal := range scriptURL.FindAllString(script, -1) {
					var raw string
					if json.Unmarshal([]byte(literal), &raw) == nil {
						add(raw, true)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return urls
}

func NormalizeImage(b []byte, source string) (Raster, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return Raster{}, fmt.Errorf("not a supported JPEG, PNG, GIF or WebP image")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return Raster{}, fmt.Errorf("image exceeds 40 megapixels")
	}
	if format != "jpeg" && format != "png" && format != "gif" && format != "webp" {
		return Raster{}, fmt.Errorf("unsupported raster format")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return Raster{}, err
	}
	w, h := config.Width, config.Height
	if w > 1200 || h > 1200 {
		if w >= h {
			h = h * 1200 / w
			w = 1200
		} else {
			w = w * 1200 / h
			h = 1200
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = scaled
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 82}); err != nil {
		return Raster{}, err
	}
	return Raster{Bytes: out.Bytes(), MIME: "image/jpeg", SourceURL: source}, nil
}

func Extract(ctx context.Context, store *Store, id SetID) (Set, error) {
	return ExtractTo(ctx, store, id, MaxSetImages)
}
func ExtractTo(ctx context.Context, store *Store, id SetID, target int) (Set, error) {
	x, err := store.Get(ctx, id)
	if err != nil {
		return x, err
	}
	if target < MinSetImages || target > MaxSetImages {
		return x, fmt.Errorf("image target must be 2–5")
	}
	if len(x.Images) >= target {
		return x, nil
	}
	expectedExample := x.Example
	outcome := func(state, message string) (Set, error) {
		if err := store.Outcome(ctx, id, expectedExample, state, message); err != nil {
			return x, err
		}
		return store.Get(ctx, id)
	}
	failure := func(err error) (Set, error) {
		result, saveErr := outcome("failed", err.Error())
		if saveErr != nil {
			return result, saveErr
		}
		if isRemotePaused(err) {
			return result, err
		}
		return result, nil
	}
	if !strings.HasPrefix(x.Example, "https://") {
		return outcome("missing", "No public example URL in source row; upload 2–5 images manually")
	}
	b, mime, err := fetchDocument(ctx, store, x.Example)
	if err != nil {
		return failure(err)
	}
	urls := []string{x.Example}
	if !strings.HasPrefix(mime, "image/") {
		urls = ExtractImages(string(b), x.Example)
	}
	if len(urls) == 0 {
		return outcome("failed", "No public post images found; upload 2–5 images manually")
	}
	existing := map[string]bool{}
	for _, img := range x.Images {
		existing[img.SourceURL] = true
	}
	messages := []string{}
	attempts := 0
	for _, raw := range urls {
		if len(x.Images) >= target || attempts >= MaxSetImages {
			break
		}
		if existing[raw] {
			continue
		}
		attempts++
		img := b
		if raw != x.Example || !strings.HasPrefix(mime, "image/") {
			img, _, err = fetchStored(ctx, store, raw, MaxImageBytes)
		}
		if err != nil {
			if isRemotePaused(err) {
				return failure(err)
			}
			messages = append(messages, err.Error())
			continue
		}
		raster, e := NormalizeImage(img, raw)
		if e != nil {
			messages = append(messages, e.Error())
			continue
		}
		raster.expectedExample = &expectedExample
		if err = store.AddImage(ctx, id, raster); err != nil {
			return x, err
		}
		x, err = store.Get(ctx, id)
		if err != nil {
			return x, err
		}
		existing[raw] = true
	}
	if len(x.Images) < MinSetImages {
		messages = append(messages, fmt.Sprintf("Only %d distinct public image(s) available; at least 2 required", len(x.Images)))
	}
	state := "ready"
	if len(x.Images) < MinSetImages {
		state = "failed"
	}
	return outcome(state, strings.Join(messages, "; "))
}

func SheetURL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Host != "docs.google.com" || u.User != nil {
		return "", "", fmt.Errorf("use a docs.google.com spreadsheet URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "spreadsheets" || parts[1] != "d" || !regexp.MustCompile(`^[a-zA-Z0-9_-]{20,200}$`).MatchString(parts[2]) {
		return "", "", fmt.Errorf("invalid spreadsheet ID")
	}
	gid := u.Query().Get("gid")
	if gid == "" {
		fragment, _ := url.ParseQuery(u.Fragment)
		gid = fragment.Get("gid")
	}
	if gid == "" {
		gid = "0"
	}
	if !regexp.MustCompile(`^\d{1,12}$`).MatchString(gid) {
		return "", "", fmt.Errorf("invalid sheet gid")
	}
	return "https://docs.google.com/spreadsheets/d/" + parts[2] + "/export?format=csv&gid=" + gid, "sheet:" + parts[2] + ":" + gid, nil
}
