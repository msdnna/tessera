package handlers

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"tessera/internal/db"
	"tessera/middleware"
)

// ackDTO is one per-user acknowledgement: an opaque key the client owns
// (whatsnew:<version>, spotlight:<feature>, getstarted:<step>) plus when it was
// first set.
type ackDTO struct {
	Key   string    `json:"key"`
	AckAt time.Time `json:"ack_at"`
}

// maxAckKeyLen bounds the opaque key so a malformed client can't write unbounded
// rows. Real keys are short (a version string or a feature slug).
const maxAckKeyLen = 200

// ListMyAcknowledgements returns every key the caller has acknowledged. The
// client compares these against the current build to decide which one-shot
// affordances (What's-New modal, sidebar spotlights, onboarding) still need showing.
func (h *API) ListMyAcknowledgements(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	rows, err := h.q.ListUserAcknowledgements(c, uid)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]ackDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, ackDTO{Key: r.Key, AckAt: r.AckAt})
	}
	c.JSON(http.StatusOK, out)
}

// AcknowledgeMe records that the caller has seen/dismissed the given key. The
// write is idempotent — pressing "Понятно" twice keeps the first-seen timestamp.
func (h *API) AcknowledgeMe(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := strings.TrimSpace(req.Key)
	// Characters, not bytes — the bound is stated in characters and is compared
	// against a key the client composed. Measured with len() a Cyrillic key hit
	// the limit at 100 characters, and the refusal is invisible: the affordance
	// the flag was meant to retire simply comes back on the next launch (#2932).
	if key == "" || utf8.RuneCountInString(key) > maxAckKeyLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key must be 1-200 chars"})
		return
	}
	row, err := h.q.UpsertUserAcknowledgement(c, db.UpsertUserAcknowledgementParams{UserID: uid, Key: key})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, ackDTO{Key: row.Key, AckAt: row.AckAt})
}
