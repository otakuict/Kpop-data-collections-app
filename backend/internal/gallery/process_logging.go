package gallery

import (
	"context"
	"log"
	"net/url"
	"time"
)

// Each entry measures a completed stage, including waits in remote fetches.
func processFinished(ctx context.Context, stage string, setID SetID, imageIndex int, source string, started time.Time, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	location := ""
	if u, parseErr := url.Parse(source); parseErr == nil {
		location = logURL(u)
	}
	log.Printf("process.finished id=%s stage=%s set=%d image=%d url=%q status=%s duration_seconds=%.3f error=%q", requestID(ctx), stage, setID, imageIndex, location, status, time.Since(started).Seconds(), logError(err))
}
