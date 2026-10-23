package gallery

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"
)

const presenceTTL = 60 * time.Second

type presenceTracker struct {
	store         *Store
	countryHeader string
	now           func() time.Time
}

func newPresenceTracker(store *Store) *presenceTracker {
	return &presenceTracker{store: store, now: time.Now}
}

func presenceCountry(c *gin.Context, header string) string {
	if header == "" {
		return ""
	}
	country := strings.ToUpper(strings.TrimSpace(c.GetHeader(header)))
	if len(country) != 2 {
		return ""
	}
	region, err := language.ParseRegion(country)
	if err != nil || !region.IsCountry() {
		return ""
	}
	return country
}

func (p *presenceTracker) heartbeat(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetHeader("X-Gallery-Analytics-Consent") != "accepted" {
		clearPresenceCookie(c)
		c.Status(http.StatusNoContent)
		return
	}
	now := p.now()
	id, _ := c.Cookie("gallery_visitor")
	decoded, decodeErr := hex.DecodeString(id)
	if decodeErr != nil || len(decoded) != 16 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "presence unavailable"})
			return
		}
		id = hex.EncodeToString(random[:])
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie("gallery_visitor", id, 0, "/api/presence", "", false, true)
	}
	if err := p.recordHeartbeat(c, id, now); err != nil {
		log.Printf("presence unavailable: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "presence unavailable"})
		return
	}
	online, err := p.store.onlineVisitors(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "presence unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"online": online})
}

func (p *presenceTracker) recordHeartbeat(c *gin.Context, visitor string, now time.Time) error {
	if err := p.store.pruneUsage(c.Request.Context(), now); err != nil {
		return err
	}
	tx, err := p.store.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var visitID, lastSeen int64
	err = tx.QueryRowContext(c.Request.Context(), `SELECT id,last_seen FROM usage_visits WHERE visitor_id=? ORDER BY last_seen DESC,id DESC LIMIT 1`, visitor).Scan(&visitID, &lastSeen)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows || now.Unix()-lastSeen >= int64(presenceTTL.Seconds()) {
		_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO usage_visits(visitor_id,country,started_at,last_seen) VALUES(?,?,?,?)`, visitor, presenceCountry(c, p.countryHeader), now.Unix(), now.Unix())
	} else {
		_, err = tx.ExecContext(c.Request.Context(), `UPDATE usage_visits SET last_seen=MAX(last_seen,?) WHERE id=?`, now.Unix(), visitID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func clearPresenceCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("gallery_visitor", "", -1, "/api/presence", "", false, true)
}

func withdrawPresence(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	clearPresenceCookie(c)
	c.Status(http.StatusNoContent)
}
