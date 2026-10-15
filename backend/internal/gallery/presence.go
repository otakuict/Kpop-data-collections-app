package gallery

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const presenceTTL = 60 * time.Second

type presenceTracker struct {
	mu       sync.Mutex
	visitors map[string]time.Time
	now      func() time.Time
}

func newPresenceTracker() *presenceTracker {
	return &presenceTracker{visitors: make(map[string]time.Time), now: time.Now}
}

func (p *presenceTracker) heartbeat(c *gin.Context) {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, seen := range p.visitors {
		if now.Sub(seen) >= presenceTTL {
			delete(p.visitors, id)
		}
	}
	id, err := c.Cookie("gallery_visitor")
	if err != nil || p.visitors[id].IsZero() {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "presence unavailable"})
			return
		}
		id = hex.EncodeToString(random[:])
		c.SetSameSite(http.SameSiteStrictMode)
		c.SetCookie("gallery_visitor", id, 0, "/api/presence", "", false, true)
	}
	p.visitors[id] = now
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"online": len(p.visitors)})
}
