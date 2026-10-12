package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/confsched"
	"tessera/internal/db"
	"tessera/middleware"
)

// A meeting's plan beyond its title and start (#3034): how long it runs and how
// it repeats (#3052), the time zone the repeat keeps its clock in, and the stage
// it belongs to (#3053).
//
// The conference PATCH has always been a full replace, and clients that predate
// these fields (Android before its parity release, an old tab) send the request
// without them. A plain full replace would read their absence as "clear": every
// edit of a title from an old client would quietly drop the repeat and the stage.
// So these four are tri-state — absent keeps, null clears, a value sets — the
// JSON tri-state PATCH the project uses for nullable fields.

// optField is one tri-state JSON field: Set=false when the key is absent, Null
// when it is an explicit null, otherwise Value.
type optField[T any] struct {
	Set   bool
	Null  bool
	Value T
}

// UnmarshalJSON runs only for a key that is present — including an explicit
// null, which encoding/json hands to an Unmarshaler as the literal.
func (o *optField[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

// conferencePlanReq is the part of a create/update body this file owns.
type conferencePlanReq struct {
	DurationMinutes optField[int32]     `json:"duration_minutes"`
	Recurrence      optField[string]    `json:"recurrence"`
	Timezone        optField[string]    `json:"timezone"`
	MilestoneID     optField[uuid.UUID] `json:"milestone_id"`
}

// conferencePlan is the resolved state the row will hold.
type conferencePlan struct {
	duration   *int32
	recurrence string
	timezone   string
	milestone  *uuid.UUID
}

// planFrom starts a plan from what a conference already holds (for an update) —
// a zero conference gives the defaults of a new one.
func planFrom(conf db.Conference) conferencePlan {
	return conferencePlan{
		duration:   conf.DurationMinutes,
		recurrence: conf.Recurrence,
		timezone:   conf.Timezone,
		milestone:  conf.MilestoneID,
	}
}

// resolvePlan applies the request onto `base`, validates it against the
// workspace and the planned start, and writes a 400/403 and returns false when
// it does not hold.
func (h *API) resolvePlan(c *gin.Context, wsID uuid.UUID, base conferencePlan, req conferencePlanReq, scheduledAt *time.Time) (conferencePlan, bool) {
	p := base
	bad := func(msg string) (conferencePlan, bool) {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return conferencePlan{}, false
	}
	if req.DurationMinutes.Set {
		if req.DurationMinutes.Null {
			p.duration = nil
		} else {
			d := req.DurationMinutes.Value
			if d < 1 || d > confsched.MaxDuration {
				return bad("duration_minutes must be between 1 and 1440")
			}
			p.duration = &d
		}
	}
	if req.Recurrence.Set {
		p.recurrence = ""
		if !req.Recurrence.Null {
			p.recurrence = req.Recurrence.Value
		}
		if !confsched.Valid(p.recurrence) {
			return bad("invalid recurrence")
		}
	}
	if req.Timezone.Set {
		p.timezone = ""
		if !req.Timezone.Null && req.Timezone.Value != "" {
			if _, err := time.LoadLocation(req.Timezone.Value); err != nil {
				return bad("invalid timezone")
			}
			p.timezone = req.Timezone.Value
		}
	}
	// A repeat counts from the planned start; without one there is nothing to
	// repeat, and accepting it would store a rule no screen can show.
	if p.recurrence != confsched.None && scheduledAt == nil {
		return bad("a repeating meeting needs scheduled_at")
	}
	// The repeat keeps its clock in a zone. The client sends the reader's; an
	// old one sends none, and then the author's profile zone is the best guess
	// at what "11:00" meant.
	if p.recurrence != confsched.None && p.timezone == "" {
		if prefs, err := h.q.GetUserPreferences(c, middleware.CurrentUser(c)); err == nil {
			if _, ok := loadLocation(prefs.Timezone); ok {
				p.timezone = prefs.Timezone
			}
		}
	}
	if req.MilestoneID.Set {
		if req.MilestoneID.Null {
			p.milestone = nil
		} else {
			// Same rule as a task's stage: only one from this workspace. The stage
			// page lists the meetings tied to it, and a link from another workspace
			// would put this call in front of a team that was never in it.
			if _, err := h.sameWorkspaceMilestone(c, req.MilestoneID.Value, wsID); err != nil {
				respondOpError(c, err)
				return conferencePlan{}, false
			}
			id := req.MilestoneID.Value
			p.milestone = &id
		}
	}
	return p, true
}

// nextAt is the occurrence the lists show (#3052): the planned start for a
// one-off, the next not-yet-finished one for a repeat. Nil without a start.
func nextAt(scheduled *time.Time, rule, tz string, duration *int32, now time.Time) *time.Time {
	if scheduled == nil {
		return nil
	}
	var length time.Duration
	if duration != nil {
		length = time.Duration(*duration) * time.Minute
	}
	t := confsched.Next(*scheduled, rule, confsched.Location(tz), length, now)
	return &t
}

// conferenceListItem is a list row with the computed next occurrence.
type conferenceListItem struct {
	db.ListConferencesRow
	NextAt *time.Time `json:"next_at"`
}

func listItems(rows []db.ListConferencesRow, now time.Time) []conferenceListItem {
	out := make([]conferenceListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, conferenceListItem{
			ListConferencesRow: r,
			NextAt:             nextAt(r.ScheduledAt, r.Recurrence, r.Timezone, r.DurationMinutes, now),
		})
	}
	return out
}

// conferenceView is one conference as the call screen reads it: the row, the
// next occurrence, the stage it is tied to and the board a task typed into its
// chat lands on by default (#3055).
type conferenceView struct {
	db.Conference
	NextAt               *time.Time `json:"next_at"`
	MilestoneTitle       *string    `json:"milestone_title"`
	MilestoneSlug        *string    `json:"milestone_slug"`
	MilestoneProjectSlug *string    `json:"milestone_project_slug"`
	DefaultBoardID       *uuid.UUID `json:"default_board_id"`
}

// viewConference joins the refs in. A failed read degrades to the bare row —
// the call is still usable without its stage pill.
func (h *API) viewConference(c *gin.Context, conf db.Conference) conferenceView {
	v := conferenceView{
		Conference: conf,
		NextAt:     nextAt(conf.ScheduledAt, conf.Recurrence, conf.Timezone, conf.DurationMinutes, time.Now()),
	}
	refs, err := h.q.ConferenceRefs(c, conf.ID)
	if err != nil {
		return v
	}
	v.MilestoneTitle = refs.MilestoneTitle
	v.MilestoneSlug = refs.MilestoneSlug
	v.MilestoneProjectSlug = refs.MilestoneProjectSlug
	v.DefaultBoardID = refs.TaskBoardID
	if v.DefaultBoardID == nil {
		v.DefaultBoardID = refs.StageBoardID
	}
	return v
}
