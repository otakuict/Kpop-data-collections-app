package gallery

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

func registerTeraBox(admin *gin.RouterGroup, store *Store, key string, sheet *SheetSync, sheetActive *atomic.Bool) {
	service := &teraService{store: store, key: key, provider: newTeraClient(), ai: exampleFinderFromEnv(), now: time.Now}
	slot := make(chan struct{}, 1)
	exclusive := func(c *gin.Context) bool {
		select {
		case slot <- struct{}{}:
			return true
		default:
			c.JSON(409, gin.H{"error": "Another TeraBox operation is running; wait for it to finish"})
			return false
		}
	}
	admin.GET("/artists", func(c *gin.Context) {
		groups, err := store.artistCatalog(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": "Cannot load artist catalog"})
			return
		}
		c.JSON(200, gin.H{"groups": groups})
	})
	admin.POST("/artists", func(c *gin.Context) {
		var member artistMember
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16000)
		if c.ShouldBindJSON(&member) != nil {
			c.JSON(400, gin.H{"error": "Invalid artist JSON"})
			return
		}
		if err := store.saveArtistMember(c.Request.Context(), member); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"saved": true})
	})
	admin.GET("/terabox/accounts", func(c *gin.Context) {
		accounts, err := store.teraAccounts(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": "Cannot load TeraBox accounts"})
			return
		}
		for i := range accounts {
			session, sessionErr := store.teraSession(c.Request.Context(), key, accounts[i].ID)
			if sessionErr != nil {
				accounts[i].Configured = false
				continue
			}
			accounts[i].Host = session.Host
			if accounts[i].Host == "" {
				accounts[i].Host = "www.terabox.com"
			}
		}
		c.JSON(200, gin.H{"accounts": accounts, "jevConfigured": service.ai.apiKey != "", "sheetSyncEnabled": sheetActive.Load()})
	})
	admin.PUT("/terabox/accounts/:account", func(c *gin.Context) {
		if !exclusive(c) {
			return
		}
		defer func() { <-slot }()
		id, err := strconv.Atoi(c.Param("account"))
		if err != nil || id < 1 || id > 2 {
			c.JSON(400, gin.H{"error": "Account must be 1 or 2"})
			return
		}
		var input struct {
			Label  string `json:"label"`
			Root   string `json:"root"`
			Source string `json:"source"`
			teraSession
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24000)
		if c.ShouldBindJSON(&input) != nil {
			c.JSON(400, gin.H{"error": "Invalid session JSON"})
			return
		}
		validateTeraSessionInput(&input.teraSession)
		a := teraAccount{ID: id, Label: input.Label, Root: input.Root, Source: input.Source}
		if err = store.saveTeraAccount(c.Request.Context(), key, a, input.teraSession); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"saved": true})
	})
	admin.POST("/terabox/preview", func(c *gin.Context) {
		if !exclusive(c) {
			return
		}
		defer func() { <-slot }()
		var input struct {
			AccountID int `json:"accountId"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		if c.ShouldBindJSON(&input) != nil || input.AccountID < 1 || input.AccountID > 2 {
			c.JSON(400, gin.H{"error": "Choose account 1 or 2"})
			return
		}
		if err := store.reconcileTeraImports(c.Request.Context()); err != nil {
			c.JSON(500, gin.H{"error": "Cannot reconcile previous imports"})
			return
		}
		preview, err := service.preview(c.Request.Context(), input.AccountID)
		if err != nil {
			log.Printf("terabox.preview.failed id=%s account=%d error=%q", requestID(c.Request.Context()), input.AccountID, err.Error())
			var rejected *teraSessionError
			if errors.As(err, &rejected) {
				c.JSON(422, gin.H{"error": err.Error(), "code": "TERABOX_SESSION_REJECTED", "upstreamErrno": -6})
				return
			}
			c.JSON(502, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, preview)
	})
	admin.POST("/terabox/commit", func(c *gin.Context) {
		if !exclusive(c) {
			return
		}
		defer func() { <-slot }()
		var input teraImportRequest
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 512<<10)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			c.JSON(400, gin.H{"error": "Invalid confirmation JSON"})
			return
		}
		results, err := service.commit(c.Request.Context(), sheet, sheetActive.Load(), input)
		if err != nil {
			c.JSON(409, gin.H{"error": err.Error(), "results": results})
			return
		}
		c.JSON(200, gin.H{"results": results})
	})
	admin.POST("/terabox/recover", func(c *gin.Context) {
		if !exclusive(c) {
			return
		}
		defer func() { <-slot }()
		var input struct {
			AccountID int `json:"accountId"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		if c.ShouldBindJSON(&input) != nil || input.AccountID < 1 || input.AccountID > 2 {
			c.JSON(400, gin.H{"error": "Choose account 1 or 2"})
			return
		}
		results, err := service.recover(c.Request.Context(), sheet, sheetActive.Load(), input.AccountID)
		if err != nil {
			c.JSON(502, gin.H{"error": "Cannot verify the live Google Sheet; pending receipts were retained"})
			return
		}
		c.JSON(200, gin.H{"results": results})
	})
}
