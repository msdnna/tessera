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

// ackDTO is one per-user acknowledgement: an opaque key the client owns plus
// when it was first set. The namespaces in use (the client is the authority on
// what they mean; the server only stores, lists and clears them):
//
//	whatsnew:web:<version>     the «Что нового» card of a web release was read
//	spotlight:<key>            a spotlight hint was dismissed (shared web/Android)
//	getstarted:…               the «Начало работы» tour: done/skipped, chapters
//	                           (getstarted:ch:<id>), checklist results
//	                           (getstarted:item:<id>[:self]), the hidden card
//	hints:off:<kind>           a kind of hint switched off in Settings → «Подсказки»
//	                           (#3101): its presence is the switch being off
//	helpful:<slug>:<lang>:…    «Статья помогла?» votes (#3071)
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

// maxAckPrefixLen mirrors maxAckKeyLen: a prefix is the head of a key.
const maxAckPrefixLen = maxAckKeyLen

// ForgetMyAcknowledgements clears flags (#3100): `?key=` one exact key («Показать
// снова» on a single hint, a switch in Settings → «Подсказки» turned back on), or
// `?prefix=` a whole namespace («Показать подсказки заново» = spotlight:,
// «Начать с начала» = getstarted:). Exactly one of the two.
//
// A prefix has to name a namespace — non-empty and ending in «:» — so a client
// slip can't turn «reset the hints» into «forget everything this account has
// ever seen» (a blank or one-letter prefix would match every key). Clearing is
// idempotent: nothing to clear answers 200 with deleted 0, the same as the POST
// answers a repeated ack.
func (h *API) ForgetMyAcknowledgements(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	key := strings.TrimSpace(c.Query("key"))
	prefix := strings.TrimSpace(c.Query("prefix"))
	if (key == "") == (prefix == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pass exactly one of key or prefix"})
		return
	}
	var (
		n   int64
		err error
	)
	if key != "" {
		if utf8.RuneCountInString(key) > maxAckKeyLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key must be 1-200 chars"})
			return
		}
		n, err = h.q.DeleteUserAcknowledgement(c, db.DeleteUserAcknowledgementParams{UserID: uid, Key: key})
	} else {
		if !strings.HasSuffix(prefix, ":") || utf8.RuneCountInString(prefix) < 2 ||
			utf8.RuneCountInString(prefix) > maxAckPrefixLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prefix must name a namespace, e.g. spotlight:"})
			return
		}
		n, err = h.q.DeleteUserAcknowledgementsByPrefix(c, db.DeleteUserAcknowledgementsByPrefixParams{UserID: uid, Prefix: prefix})
	}
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}
