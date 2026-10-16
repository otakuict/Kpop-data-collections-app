package gallery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
)

type requestIDKey struct{}

var urlInError = regexp.MustCompile(`https?://[^\s"'<>]+`)

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func ensureRequestID(ctx context.Context) context.Context {
	if requestID(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, newRequestID())
}

func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

// Log only the URL's location. Query strings, headers, and bodies can contain
// administrator tokens, signed URLs, or user supplied data.
func logURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.Host == "" {
		return path
	}
	return u.Scheme + "://" + u.Host + path
}

func logError(err error) string {
	if err == nil {
		return ""
	}
	return urlInError.ReplaceAllStringFunc(err.Error(), func(raw string) string {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			return "[invalid URL]"
		}
		return logURL(u)
	})
}

func httpRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := newRequestID()
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
		c.Header("X-Request-ID", id)
		started := time.Now()
		log.Printf("http.in.request id=%s method=%s url=%q", id, c.Request.Method, logURL(c.Request.URL))
		c.Next()
		log.Printf("http.in.response id=%s method=%s route=%q status=%d bytes=%d duration_ms=%d", id, c.Request.Method, c.FullPath(), c.Writer.Status(), c.Writer.Size(), time.Since(started).Milliseconds())
	}
}

type loggingTransport struct {
	base http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := requestID(req.Context())
	target := logURL(req.URL)
	started := time.Now()
	log.Printf("http.out.request id=%s method=%s url=%q", id, req.Method, target)
	response, err := t.base.RoundTrip(req)
	if err != nil {
		log.Printf("http.out.error id=%s method=%s url=%q duration_ms=%d error=%q", id, req.Method, target, time.Since(started).Milliseconds(), logError(err))
		return response, err
	}
	log.Printf("http.out.response id=%s method=%s url=%q status=%d duration_ms=%d", id, req.Method, target, response.StatusCode, time.Since(started).Milliseconds())
	return response, nil
}
