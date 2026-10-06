package gallery

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Router(store *Store, token string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	})
	fail := func(c *gin.Context, err error) {
		code := 500
		message := "Database operation failed"
		if errors.Is(err, ErrAlbumFull) || errors.Is(err, ErrAlbumIncomplete) {
			code = 400
			message = err.Error()
		}
		var paused *RemotePausedError
		if errors.As(err, &paused) {
			code = 429
			message = paused.Error()
			c.Header("Retry-After", strconv.Itoa(max(1, int(time.Until(paused.Until).Seconds()))))
			c.JSON(code, gin.H{"error": message, "pausedUntil": paused.Until.UTC().Format(time.RFC3339)})
			return
		}
		if errors.Is(err, ErrSourceChanged) {
			code = 409
			message = err.Error()
		}
		if err == sql.ErrNoRows {
			code = 404
			message = "Set or image not found"
		}
		c.JSON(code, gin.H{"error": message})
	}
	r.GET("/api/health", func(c *gin.Context) {
		if err := store.db.PingContext(c.Request.Context()); err != nil {
			c.JSON(503, gin.H{"error": "database unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/api/facets", func(c *gin.Context) {
		f, err := store.Facets(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, f)
	})
	r.GET("/api/sets", func(c *gin.Context) {
		page, e := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "24"))
		if e != nil || e2 != nil || page < 1 || page > 100000 || limit < 1 || limit > 200 {
			c.JSON(400, gin.H{"error": "invalid page or limit"})
			return
		}
		f := Filter{Query: c.Query("q"), Group: c.Query("group"), From: c.Query("from"), To: c.Query("to"), Sort: c.Query("sort"), Page: page, Limit: limit}
		for _, date := range []string{f.From, f.To} {
			if date != "" {
				if _, err := time.Parse("2006-01-02", date); err != nil {
					c.JSON(400, gin.H{"error": "dates must be YYYY-MM-DD"})
					return
				}
			}
		}
		if len(f.Query) > 500 || len(f.Group) > 150 || (f.From != "" && f.To != "" && f.From > f.To) {
			c.JSON(400, gin.H{"error": "invalid filters or date range"})
			return
		}
		p, err := store.List(c.Request.Context(), f)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, p)
	})
	r.GET("/api/sets/:id", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		x, err := store.Get(c.Request.Context(), id)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, x)
	})
	r.GET("/api/images/:id", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		b, mime, err := store.ImageBytes(c.Request.Context(), ImageID(id))
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(200, mime, b)
	})
	admin := r.Group("/api/admin")
	admin.Use(func(c *gin.Context) {
		supplied := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if len(token) < 24 || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 {
			c.AbortWithStatusJSON(401, gin.H{"error": "A valid administrator token is required"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
		c.Next()
	})
	admin.GET("/ingestion", func(c *gin.Context) {
		status, err := store.IngestionStatus(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, status)
	})
	admin.GET("/status", func(c *gin.Context) { c.JSON(200, gin.H{"authenticated": true}) })
	save := func(c *gin.Context, id SetID) {
		var d SetDraft
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		if err := c.ShouldBindJSON(&d); err != nil {
			c.JSON(400, gin.H{"error": "invalid metadata JSON"})
			return
		}
		if err := d.Validate(); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		x, err := store.Save(c.Request.Context(), id, d)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, x)
	}
	admin.POST("/sets", func(c *gin.Context) { save(c, 0) })
	admin.PUT("/sets/:id", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		save(c, id)
	})
	admin.POST("/sets/:id/extract", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		target, err := strconv.Atoi(c.DefaultQuery("target", "5"))
		if err != nil || target < MinSetImages || target > MaxSetImages {
			c.JSON(400, gin.H{"error": "image target must be 2–5"})
			return
		}
		x, err := ExtractTo(c.Request.Context(), store, id, target)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, x)
	})
	admin.POST("/sets/:id/images", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if _, err := store.Get(c.Request.Context(), id); err != nil {
			fail(c, err)
			return
		}
		if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
			c.JSON(400, gin.H{"error": "invalid or oversized image upload"})
			return
		}
		defer c.Request.MultipartForm.RemoveAll()
		files := c.Request.MultipartForm.File["images"]
		if len(files) == 0 || len(files) > MaxSetImages {
			c.JSON(400, gin.H{"error": "choose up to 5 image files; each set needs 2–5 distinct images"})
			return
		}
		rasters := []Raster{}
		for _, f := range files {
			if f.Size > MaxImageBytes {
				c.JSON(400, gin.H{"error": "each image must be at most 12 MiB"})
				return
			}
			reader, err := f.Open()
			if err != nil {
				c.JSON(400, gin.H{"error": "cannot read uploaded file"})
				return
			}
			b, err := io.ReadAll(io.LimitReader(reader, MaxImageBytes+1))
			reader.Close()
			if err != nil || len(b) > MaxImageBytes {
				c.JSON(400, gin.H{"error": "cannot read uploaded image"})
				return
			}
			r, err := NormalizeImage(b, "")
			if err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			rasters = append(rasters, r)
		}
		if err := store.AddImages(c.Request.Context(), id, rasters); err != nil {
			fail(c, err)
			return
		}
		x, err := store.Get(c.Request.Context(), id)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, x)
	})
	admin.POST("/import", func(c *gin.Context) {
		var sheetURL string
		var b []byte
		var source string
		var err error
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			if err = c.Request.ParseMultipartForm(1 << 20); err != nil {
				c.JSON(400, gin.H{"error": "invalid CSV upload"})
				return
			}
			defer c.Request.MultipartForm.RemoveAll()
			sheetURL = c.PostForm("sheetUrl")
			if sheetURL == "" {
				sheetURL = DefaultSheetURL
			}
			_, source, err = SheetURL(sheetURL)
			if err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			file, _, e := c.Request.FormFile("csv")
			if e != nil {
				c.JSON(400, gin.H{"error": "select a CSV file"})
				return
			}
			b, err = io.ReadAll(io.LimitReader(file, 5<<20+1))
			file.Close()
		} else {
			var request struct {
				SheetURL string `json:"sheetUrl"`
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
			if err = c.ShouldBindJSON(&request); err != nil {
				c.JSON(400, gin.H{"error": "sheetUrl is required"})
				return
			}
			sheetURL = request.SheetURL
			var remote string
			remote, source, err = SheetURL(sheetURL)
			if err == nil {
				b, _, err = Fetch(c.Request.Context(), remote, 5<<20)
			}
		}
		if err != nil {
			c.JSON(400, gin.H{"error": fmt.Sprintf("Sheet import failed: %v", err)})
			return
		}
		if len(b) > 5<<20 {
			c.JSON(400, gin.H{"error": "CSV exceeds 5 MiB"})
			return
		}
		drafts, err := ParseSheet(bytes.NewReader(b), source)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		report, err := store.Import(c.Request.Context(), drafts)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(200, report)
	})
	return r
}
func parseID(c *gin.Context) (SetID, bool) {
	n, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || n <= 0 {
		c.JSON(400, gin.H{"error": "invalid ID"})
		return 0, false
	}
	return SetID(n), true
}

const DefaultSheetURL = "https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/edit?gid=0"
