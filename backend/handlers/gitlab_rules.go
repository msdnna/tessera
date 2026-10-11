package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/gitlab"
)

// Label rules — counters and the dry run (#3087).
//
// The «Правила» section of the GitLab window shows, by each rule, how many of the
// binding's tasks it decides something for, and a «Проверить лейбл» field that
// walks one label through the rules. Both run on the rules AS EDITED (the form
// sends them), not as stored, so a rule switched off or reordered a second ago is
// what the numbers describe — they are what saving would do. The labels come from
// the snapshot every pull records on a link; a link synced before the labels were
// recorded is counted once its issue is pulled again (`recorded` says how many
// already are).

// rulesPreviewRequest is the body of the preview: the edited rules, the binding
// whose tasks to count (optional — a binding not saved yet has none) and the
// label to check (optional).
type rulesPreviewRequest struct {
	IntegrationID *uuid.UUID      `json:"integration_id"`
	LabelRules    json.RawMessage `json:"label_rules"`
	Label         string          `json:"label"`
}

// rulesCheckView is the answer about one label.
type rulesCheckView struct {
	Outcome gitlab.LabelOutcome `json:"outcome"`
	// Tasks is how many of the binding's tasks carry the label now.
	Tasks int `json:"tasks"`
}

// PreviewGitlabRules counts the binding's tasks by rule and checks a label
// against the edited rules. Read-only: members may look, as they may read the
// config itself.
func (h *API) PreviewGitlabRules(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req rulesPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	rules := parseRules(req.LabelRules)

	// The label sets of the binding's live tasks, as last pulled.
	var sets [][]gitlab.Label
	recorded, linked := 0, 0
	if req.IntegrationID != nil {
		integ, err := h.q.GetGitlabIntegration(c, *req.IntegrationID)
		if err != nil || integ.WorkspaceID != wsID {
			c.JSON(http.StatusNotFound, gin.H{"error": "integration not found"})
			return
		}
		snaps, err := h.q.LinkSnapshotsForIntegration(c, integ.ID)
		if err != nil {
			fail(c, err)
			return
		}
		linked = len(snaps)
		for _, raw := range snaps {
			var s struct {
				Labels *[]string `json:"labels"`
			}
			if json.Unmarshal(raw, &s) != nil || s.Labels == nil {
				continue // taken before labels were recorded
			}
			recorded++
			set := make([]gitlab.Label, 0, len(*s.Labels))
			for _, t := range *s.Labels {
				set = append(set, gitlab.Label{Title: t})
			}
			sets = append(sets, set)
		}
	}

	resp := gin.H{
		"hits":     rules.RuleHits(sets),
		"linked":   linked,
		"recorded": recorded,
	}
	if label := strings.TrimSpace(req.Label); label != "" {
		outs := rules.Explain([]gitlab.Label{{Title: label}})
		check := rulesCheckView{}
		if len(outs) > 0 {
			check.Outcome = outs[0]
		}
		for _, set := range sets {
			for _, l := range set {
				if l.Title == label {
					check.Tasks++
					break
				}
			}
		}
		resp["check"] = check
	}
	c.JSON(http.StatusOK, resp)
}
