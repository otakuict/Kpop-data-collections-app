package gallery

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const MaxUploadImageBytes = 15 << 20
const maxUploadImagePixels = 12_000_000
const maxUploadBatchPixels = 12_000_000

// A decoded image can be much larger than its uploaded bytes. Keep separate
// manual uploads from decoding large images at the same time.
var imageUploadSlot = make(chan struct{}, 1)

func Router(store *Store, token string) *gin.Engine {
	r := gin.New()
	presence := newPresenceTracker()
	r.Use(httpRequestLogger())
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
		if errors.Is(err, ErrNoImageSource) {
			code = 400
			message = err.Error()
		}
		if errors.Is(err, ErrImageFetch) {
			code = 502
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
		log.Printf("http.handler.error id=%s status=%d error=%q", requestID(c.Request.Context()), code, logError(err))
		c.JSON(code, gin.H{"error": message})
	}
	r.GET("/api/health", func(c *gin.Context) {
		if err := store.db.PingContext(c.Request.Context()); err != nil {
			c.JSON(503, gin.H{"error": "database unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.POST("/api/presence", presence.heartbeat)
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
	r.POST("/api/admin/session", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !validAdminToken(adminBearer(c.GetHeader("Authorization")), token) {
			c.JSON(401, gin.H{"error": "A valid administrator token is required"})
			return
		}
		c.JSON(200, gin.H{"token": issueAdminJWT(token, time.Now()), "expiresIn": int(adminSessionLifetime.Seconds())})
	})
	admin := r.Group("/api/admin")
	admin.Use(func(c *gin.Context) {
		supplied := adminBearer(c.GetHeader("Authorization"))
		if !validAdminToken(supplied, token) && !validAdminJWT(supplied, token, time.Now()) {
			c.AbortWithStatusJSON(401, gin.H{"error": "A valid administrator token is required"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 80<<20)
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
		var payload struct {
			SetDraft
			Style *SetStyle `json:"style"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(400, gin.H{"error": "invalid metadata JSON"})
			return
		}
		if err := payload.SetDraft.Validate(); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if payload.Style != nil {
			if err := payload.Style.Validate(); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
		}
		x, err := store.SaveStyled(c.Request.Context(), id, payload.SetDraft, payload.Style)
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
	admin.POST("/sets/:id/reload-images", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		x, err := ReloadSetImages(c.Request.Context(), store, id)
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
		select {
		case imageUploadSlot <- struct{}{}:
			defer func() { <-imageUploadSlot }()
		case <-c.Request.Context().Done():
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
		if err := validateUploadFiles(files); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		rasters := []Raster{}
		for _, f := range files {
			reader, err := f.Open()
			if err != nil {
				c.JSON(400, gin.H{"error": "cannot read uploaded file"})
				return
			}
			b, err := io.ReadAll(io.LimitReader(reader, MaxUploadImageBytes+1))
			reader.Close()
			if err != nil || len(b) > MaxUploadImageBytes {
				c.JSON(400, gin.H{"error": "cannot read uploaded image"})
				return
			}
			r, err := normalizeImage(b, "", maxUploadImagePixels)
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
	admin.POST("/sets/:id/images/:imageID/rotate", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		imageID, err := strconv.ParseInt(c.Param("imageID"), 10, 64)
		if err != nil || imageID <= 0 {
			c.JSON(400, gin.H{"error": "invalid image ID"})
			return
		}
		if err := store.RotateImage(c.Request.Context(), id, ImageID(imageID)); err != nil {
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
	importSheet := func(preview bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			var sheetURL string
			var b []byte
			var source string
			var imagePolicy ImageImportPolicy
			var err error
			if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
				if err = c.Request.ParseMultipartForm(1 << 20); err != nil {
					c.JSON(400, gin.H{"error": "invalid CSV upload"})
					return
				}
				defer c.Request.MultipartForm.RemoveAll()
				sheetURL = c.PostForm("sheetUrl")
				imagePolicy = ImageImportPolicy(c.PostForm("imagePolicy"))
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
					SheetURL    string            `json:"sheetUrl"`
					ImagePolicy ImageImportPolicy `json:"imagePolicy"`
				}
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
				if err = c.ShouldBindJSON(&request); err != nil {
					c.JSON(400, gin.H{"error": "sheetUrl is required"})
					return
				}
				sheetURL = request.SheetURL
				imagePolicy = request.ImagePolicy
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
			if imagePolicy == "" {
				imagePolicy = KeepExistingImages
			}
			if !imagePolicy.valid() {
				c.JSON(400, gin.H{"error": "image policy must be keep or replace"})
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
			var report ImportReport
			if preview {
				report, err = store.PreviewImportWithImagePolicy(c.Request.Context(), drafts, imagePolicy)
			} else {
				report, err = store.ImportWithImagePolicy(c.Request.Context(), drafts, imagePolicy)
			}
			if err != nil {
				fail(c, err)
				return
			}
			if preview {
				c.JSON(200, gin.H{"report": report, "csv": string(b), "sheetUrl": sheetURL, "imagePolicy": imagePolicy})
				return
			}
			c.JSON(200, report)
		}
	}
	admin.POST("/import/preview", importSheet(true))
	admin.POST("/import", importSheet(false))
	return r
}

func validateUploadFiles(files []*multipart.FileHeader) error {
	var totalPixels int64
	for _, file := range files {
		if file.Size > MaxUploadImageBytes {
			return fmt.Errorf("each image must be at most 15 MiB")
		}
		reader, err := file.Open()
		if err != nil {
			return fmt.Errorf("cannot read uploaded file")
		}
		config, format, decodeErr := image.DecodeConfig(reader)
		reader.Close()
		if decodeErr != nil || (format != "jpeg" && format != "png" && format != "gif" && format != "webp") {
			return fmt.Errorf("not a supported JPEG, PNG, GIF or WebP image")
		}
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxUploadImagePixels/int64(config.Height) {
			return fmt.Errorf("each image must be at most 12 megapixels")
		}
		totalPixels += int64(config.Width) * int64(config.Height)
		if totalPixels > maxUploadBatchPixels {
			return fmt.Errorf("combined upload must be at most 12 megapixels; use the back office to resize images")
		}
	}
	return nil
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
