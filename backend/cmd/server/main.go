package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"otakuict/gallery/internal/gallery"
)

func env(key, fallback string) string {
	if x := os.Getenv(key); x != "" {
		return x
	}
	return fallback
}
func main() {
	dbPath := flag.String("db", env("DATABASE_PATH", "./data/gallery.sqlite"), "SQLite file")
	seed := flag.String("seed", env("SEED_DATABASE", "./data/seed.sqlite"), "initial SQLite snapshot")
	csvPath := flag.String("import", "", "import source CSV and exit")
	extract := flag.Bool("extract", false, "fill incomplete sets after CLI import; stops on provider cooldown")
	target := flag.Int("image-target", gallery.MinSetImages, "image count target for extraction (2–5)")
	cachedPage := flag.String("cache-page", "", "cache a previously downloaded HTML page without network requests")
	backup := flag.String("backup", "", "export a consistent SQLite snapshot and exit")
	pageURL := flag.String("page-url", "", "source URL for -cache-page")
	flag.Parse()
	if *target < gallery.MinSetImages || *target > gallery.MaxSetImages {
		log.Fatal("-image-target must be 2–5")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0750); err != nil {
		log.Fatal(err)
	}
	if *csvPath == "" {
		if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
			if err = copySeed(*seed, *dbPath); err != nil {
				log.Fatal(err)
			}
		}
	}
	store, err := gallery.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *backup != "" {
		if err = store.Backup(ctx, *backup); err != nil {
			log.Fatal(err)
		}
		log.Print("SQLite snapshot exported")
		return
	}
	if *cachedPage != "" {
		b, err := os.ReadFile(*cachedPage)
		if err != nil {
			log.Fatal(err)
		}
		if err = store.CachePage(ctx, *pageURL, b, "text/html"); err != nil {
			log.Fatal(err)
		}
		log.Print("Cached supplied HTML; no network requests sent")
		return
	}
	if *csvPath != "" {
		if err = runImport(ctx, store, *csvPath, *extract, *target); err != nil {
			log.Fatal(err)
		}
		return
	}
	token := os.Getenv("ADMIN_TOKEN")
	if len(token) < 24 {
		log.Fatal("ADMIN_TOKEN must contain at least 24 characters")
	}
	server := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: gallery.Router(store, token), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 120 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Print(err)
		}
	}()
	log.Printf("Gallery API listening on %s", server.Addr)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
func copySeed(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("initial seed missing: %w; use -import data/source.csv to initialize", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dest+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(dest+".tmp", dest)
}
func runImport(ctx context.Context, store *gallery.Store, path string, extract bool, target int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, source, _ := gallery.SheetURL(gallery.DefaultSheetURL)
	drafts, err := gallery.ParseSheet(f, source)
	if err != nil {
		return err
	}
	report, err := store.Import(ctx, drafts)
	if err != nil {
		return err
	}
	log.Printf("Metadata: %d rows, %d created, %d updated", report.Total, report.Created, report.Updated)
	if !extract {
		return nil
	}
	for _, id := range report.Candidates {
		x, err := gallery.ExtractTo(ctx, store, id, target)
		if err != nil {
			return fmt.Errorf("extraction stopped at set %d: %w", id, err)
		}
		log.Printf("set %d: %d images, %s %s", id, len(x.Images), x.ImageState, strings.ReplaceAll(x.ImageError, "\n", " "))
	}
	fcts, err := store.Facets(ctx)
	if err != nil {
		return err
	}
	log.Printf("Complete: %d sets, %d with stored images", fcts.Total, fcts.Ready)
	return nil
}
