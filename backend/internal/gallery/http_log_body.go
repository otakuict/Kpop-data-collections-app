package gallery

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const httpLogBodyLimit = 16 << 10
const redactedLogValue = "[REDACTED]"

type bodyLog struct {
	data      []byte
	truncated bool
}

func newBodyLog(data []byte, truncated bool) *bodyLog {
	if len(data) > httpLogBodyLimit {
		data = data[:httpLogBodyLimit]
		truncated = true
	}
	return &bodyLog{data: append([]byte(nil), data...), truncated: truncated}
}
func (b *bodyLog) append(data []byte) {
	remaining := httpLogBodyLimit - len(b.data)
	if len(data) > remaining {
		b.truncated = true
		data = data[:remaining]
	}
	b.data = append(b.data, data...)
}
func sensitiveLogKey(key string) bool {
	key = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	for _, part := range []string{"token", "password", "passwd", "secret", "cookie", "authorization", "credential", "signature", "assertion", "privatekey", "apikey", "session", "csrf", "jwt"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return key == "key" || key == "code" || key == "ndus" || key == "sign" || key == "sig" || key == "auth" || key == "pwd" || key == "pass" || key == "pin" || key == "randsk" || key == "sekey" || key == "keypairid"
}

var bearerInLog = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
var namedSecretInLog = regexp.MustCompile(`(?i)((?:access_token|refresh_token|jsToken|bdstoken|ndus|password|pwd|randsk|sekey|api[_-]?key|signature|secret)\s*[=:]\s*["']?)[^\s&"'<>;,]+`)

func sanitizeLogText(text string, secrets []string) string {
	text = urlInError.ReplaceAllStringFunc(text, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[invalid URL]"
		}
		parsed.User = nil
		parsed.Fragment = ""
		values := parsed.Query()
		for key, entries := range values {
			if sensitiveLogKey(key) {
				values[key] = []string{redactedLogValue}
				continue
			}
			for i, value := range entries {
				for _, secret := range secrets {
					if secret != "" {
						value = strings.ReplaceAll(value, secret, redactedLogValue)
					}
				}
				entries[i] = value
			}
		}
		parsed.RawQuery = values.Encode()
		return parsed.String()
	})
	text = bearerInLog.ReplaceAllString(text, "Bearer "+redactedLogValue)
	text = namedSecretInLog.ReplaceAllString(text, "${1}"+redactedLogValue)
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, redactedLogValue)
		}
	}
	return text
}
func sanitizeLogValue(value any, secrets []string, depth int) any {
	if depth > 32 {
		return "[omitted: nesting limit]"
	}
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			if sensitiveLogKey(key) {
				out[key] = redactedLogValue
			} else {
				out[key] = sanitizeLogValue(item, secrets, depth+1)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeLogValue(item, secrets, depth+1)
		}
		return out
	case string:
		return sanitizeLogText(v, secrets)
	default:
		return v
	}
}
func logJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[unavailable]"
	}
	if len(data) > httpLogBodyLimit {
		return "[omitted: log exceeds 16 KiB]"
	}
	return string(data)
}
func logQuery(values url.Values, secrets []string) string {
	out := map[string]any{}
	for key, value := range values {
		if sensitiveLogKey(key) {
			out[key] = redactedLogValue
		} else {
			entries := []any{}
			for _, v := range value {
				entries = append(entries, sanitizeLogText(v, secrets))
			}
			out[key] = entries
		}
	}
	return logJSON(out)
}
func logHeaders(headers http.Header, secrets []string) string {
	out := map[string]any{}
	for key, values := range headers {
		if sensitiveLogKey(key) {
			out[key] = redactedLogValue
		} else {
			entries := []any{}
			for _, v := range values {
				entries = append(entries, sanitizeLogText(v, secrets))
			}
			out[key] = entries
		}
	}
	return logJSON(out)
}
func (b *bodyLog) render(contentType string, secrets ...string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "multipart/") || strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "audio/") || mediaType == "application/octet-stream" || mediaType == "text/event-stream" {
		return "[omitted: binary, multipart or streaming body]"
	}
	if b.truncated {
		return "[omitted: body exceeds 16 KiB]"
	}
	if len(b.data) == 0 {
		return ""
	}
	if !utf8.Valid(b.data) {
		return "[omitted: binary body]"
	}
	var value any
	if json.Unmarshal(b.data, &value) == nil {
		return logJSON(sanitizeLogValue(value, secrets, 0))
	}
	if strings.Contains(mediaType, "json") {
		return "[omitted: invalid JSON body]"
	}
	if mediaType == "application/x-www-form-urlencoded" {
		values, err := url.ParseQuery(string(b.data))
		if err != nil {
			return "[omitted: invalid form body]"
		}
		return logQuery(values, secrets)
	}
	if mediaType != "" && !strings.HasPrefix(mediaType, "text/") {
		return "[omitted: unsupported body type]"
	}
	return sanitizeLogText(string(b.data), secrets)
}

type replayLogBody struct {
	io.Reader
	original io.ReadCloser
}

func (b *replayLogBody) Close() error { return b.original.Close() }
func captureLogBody(body io.ReadCloser, contentType string) (io.ReadCloser, *bodyLog) {
	capture := newBodyLog(nil, false)
	if body == nil || body == http.NoBody {
		return body, capture
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "multipart/") || strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "audio/") || mediaType == "application/octet-stream" || mediaType == "text/event-stream" {
		return body, capture
	}
	prefix, err := io.ReadAll(io.LimitReader(body, httpLogBodyLimit+1))
	capture = newBodyLog(prefix, len(prefix) > httpLogBodyLimit)
	// Replay even a partial read error so the handler/HTTP client sees it too.
	reader := io.MultiReader(bytes.NewReader(prefix), body)
	if err != nil {
		reader = io.MultiReader(bytes.NewReader(prefix), errorLogReader{err})
	}
	return &replayLogBody{Reader: reader, original: body}, capture
}

type errorLogReader struct{ err error }

func (r errorLogReader) Read([]byte) (int, error) { return 0, r.err }

func logRequestSecrets(req *http.Request, body *bodyLog) []string {
	secrets := []string{}
	for key, values := range req.URL.Query() {
		if sensitiveLogKey(key) {
			secrets = append(secrets, values...)
		}
	}
	for key, values := range req.Header {
		if sensitiveLogKey(key) {
			for _, value := range values {
				secrets = append(secrets, value)
				if strings.HasPrefix(value, "Bearer ") {
					secrets = append(secrets, strings.TrimPrefix(value, "Bearer "))
				}
			}
		}
	}
	for _, cookie := range req.Cookies() {
		if sensitiveLogKey(cookie.Name) {
			secrets = append(secrets, cookie.Value)
		}
	}
	var value any
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if sensitiveLogKey(key) {
					if s, ok := item.(string); ok {
						secrets = append(secrets, s)
					}
				} else {
					collect(item)
				}
			}
		case []any:
			for _, item := range v {
				collect(item)
			}
		}
	}
	if !body.truncated && json.Unmarshal(body.data, &value) == nil {
		collect(value)
	}
	values, _ := url.ParseQuery(string(body.data))
	for key, entries := range values {
		if sensitiveLogKey(key) {
			secrets = append(secrets, entries...)
		}
	}
	return secrets
}
