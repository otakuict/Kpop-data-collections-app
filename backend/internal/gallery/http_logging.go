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

type loggingResponseWriter struct {
	gin.ResponseWriter
	body *bodyLog
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.body.append(data[:n])
	return n, err
}
func (w *loggingResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	w.body.append([]byte(data[:n]))
	return n, err
}

func httpRequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := newRequestID()
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
		c.Header("X-Request-ID", id)
		started := time.Now()
		body, capture := captureLogBody(c.Request.Body, c.Request.Header.Get("Content-Type"))
		c.Request.Body = body
		secrets := logRequestSecrets(c.Request, capture)
		writer := &loggingResponseWriter{ResponseWriter: c.Writer, body: newBodyLog(nil, false)}
		c.Writer = writer
		log.Printf("http.in.request id=%s method=%s url=%q direction=incoming query=%q headers=%q body=%q body_truncated=%t", id, c.Request.Method, logURL(c.Request.URL), logQuery(c.Request.URL.Query(), secrets), logHeaders(c.Request.Header, secrets), capture.render(c.Request.Header.Get("Content-Type"), secrets...), capture.truncated)
		defer func() {
			log.Printf("http.in.response id=%s method=%s route=%q direction=incoming status=%d bytes=%d duration_ms=%d headers=%q body=%q body_truncated=%t", id, c.Request.Method, c.FullPath(), c.Writer.Status(), c.Writer.Size(), time.Since(started).Milliseconds(), logHeaders(c.Writer.Header(), secrets), writer.body.render(c.Writer.Header().Get("Content-Type"), secrets...), writer.body.truncated)
		}()
		c.Next()
	}
}

type loggingTransport struct{ base http.RoundTripper }

// All server HTTP clients go through this boundary, including injected test
// clients. Copy the client so request-specific wrapping cannot race callers.
func loggedDo(client *http.Client, req *http.Request) (*http.Response, error) {
	copied := *client
	if _, ok := copied.Transport.(*loggingTransport); !ok {
		base := copied.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		copied.Transport = &loggingTransport{base: base}
	}
	return copied.Do(req.WithContext(ensureRequestID(req.Context())))
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := requestID(req.Context())
	if id == "" {
		id = newRequestID()
	}
	callID := newRequestID()
	target := logURL(req.URL)
	started := time.Now()
	body, capture := captureLogBody(req.Body, req.Header.Get("Content-Type"))
	req.Body = body
	secrets := logRequestSecrets(req, capture)
	log.Printf("http.out.request id=%s method=%s url=%q direction=outgoing call_id=%s query=%q headers=%q body=%q body_truncated=%t", id, req.Method, target, callID, logQuery(req.URL.Query(), secrets), logHeaders(req.Header, secrets), capture.render(req.Header.Get("Content-Type"), secrets...), capture.truncated)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(req)
	if err != nil {
		log.Printf("http.out.error id=%s method=%s url=%q direction=outgoing call_id=%s duration_ms=%d error=%q", id, req.Method, target, callID, time.Since(started).Milliseconds(), sanitizeLogText(logError(err), secrets))
		return response, err
	}
	responseBody, responseCapture := captureLogBody(response.Body, response.Header.Get("Content-Type"))
	response.Body = responseBody
	log.Printf("http.out.response id=%s method=%s url=%q direction=outgoing call_id=%s status=%d duration_ms=%d headers=%q body=%q body_truncated=%t", id, req.Method, target, callID, response.StatusCode, time.Since(started).Milliseconds(), logHeaders(response.Header, secrets), responseCapture.render(response.Header.Get("Content-Type"), secrets...), responseCapture.truncated)
	return response, nil
}
