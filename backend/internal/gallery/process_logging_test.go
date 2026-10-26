package gallery

import (
	"bytes"
	"context"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestProcessFinishedLogsSecondsAndSafeContext(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	for _, err := range []error{nil, errors.New("download failed")} {
		processFinished(context.Background(), "image_download", 42, 2, "https://media.ganknow.com/public/PM.jpg?token=secret", time.Now().Add(-1500*time.Millisecond), err)
	}
	got := output.String()
	for _, field := range []string{"process.finished", "stage=image_download", "set=42", "image=2", "status=success", "status=error"} {
		if !strings.Contains(got, field) {
			t.Fatalf("missing %q in %s", field, got)
		}
	}
	if !regexp.MustCompile(`duration_seconds=1\.50[0-9]`).MatchString(got) {
		t.Fatalf("expected fractional seconds: %s", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("URL credentials leaked: %s", got)
	}
}

func TestExtractionFailureLogsFailedTotalEvenWhenReturnedAsSet(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	store, err := Open(filepath.Join(t.TempDir(), "timing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	set, err := store.Save(ctx, 0, SetDraft{Title: "No previews", Group: "aespa", Example: "https://ganknow.com/post/no-preview"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CachePage(ctx, set.Example, []byte("<html></html>"), "text/html"); err != nil {
		t.Fatal(err)
	}
	result, err := ExtractTo(ctx, store, set.ID, 2)
	if err != nil || result.ImageState != "failed" {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.Contains(line, "stage=extract_total ") {
			if !strings.Contains(line, "status=error") || !strings.Contains(line, "duration_seconds=") {
				t.Fatalf("incorrect total: %s", line)
			}
			return
		}
	}
	t.Fatal("missing extraction total")
}
