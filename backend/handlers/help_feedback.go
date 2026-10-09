package handlers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// helpFeedbackPrefix is the acknowledgement-key prefix of a «Статья помогла?»
// vote (#3071): helpful:<slug>:<lang>:yes|no. The web client composes it in
// utils/helpFeedback.js — the two have to agree.
const helpFeedbackPrefix = "helpful:"

// helpFeedbackRow is one article in one language with its vote totals.
type helpFeedbackRow struct {
	Slug string `json:"slug"`
	Lang string `json:"lang"`
	Yes  int64  `json:"yes"`
	No   int64  `json:"no"`
}

// parseHelpVoteKey splits helpful:<slug>:<lang>:<vote>. The slug may not hold a
// colon (help slugs are kebab-case), so the last two fields are taken from the
// right; anything that does not end in yes|no is not a vote and is skipped.
func parseHelpVoteKey(key string) (slug, lang, vote string, ok bool) {
	rest, found := strings.CutPrefix(key, helpFeedbackPrefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	if parts[2] != "yes" && parts[2] != "no" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// HelpFeedback returns the «Статья помогла?» totals per article and language
// for the administration (#3071). Global admins only; the votes themselves
// stay per-user acknowledgements and are never listed one by one.
func (h *API) HelpFeedback(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	rows, err := h.q.HelpFeedbackTotals(c)
	if err != nil {
		fail(c, err)
		return
	}
	byArticle := map[string]*helpFeedbackRow{}
	for _, r := range rows {
		slug, lang, vote, ok := parseHelpVoteKey(r.Key)
		if !ok {
			continue
		}
		id := slug + ":" + lang
		row := byArticle[id]
		if row == nil {
			row = &helpFeedbackRow{Slug: slug, Lang: lang}
			byArticle[id] = row
		}
		if vote == "yes" {
			row.Yes += r.Votes
		} else {
			row.No += r.Votes
		}
	}
	out := make([]helpFeedbackRow, 0, len(byArticle))
	for _, row := range byArticle {
		out = append(out, *row)
	}
	// Most-voted first, then by slug and language so the order is stable.
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].Yes+out[i].No, out[j].Yes+out[j].No
		if ti != tj {
			return ti > tj
		}
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		return out[i].Lang < out[j].Lang
	})
	c.JSON(http.StatusOK, out)
}
