package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
)

// The v2 notifications (#3048): a stage closed, a meeting about to start, a
// sync conflict waiting for a decision, a mention in a document's comment and in
// a meeting's chat. Each is an event (the render key, see the ev* names in
// notification_content.go), a builder here, a routing kind, and a key in every
// catalogue that renders it — notify/locale.go for the deliveries that leave
// the app, locales/*/notifications.json on the web and NotificationText.kt on
// Android.

// conferenceSoonLead is how long before a planned meeting «встреча через 5
// минут» goes out.
const conferenceSoonLead = 5 * time.Minute

// msgMilestoneClosed announces that a stage was closed, with how far it got. The
// stage carries no task, so the payload says where it lives: the project (name
// for the sentence, id and slug for the link to the stage page) and the stage's
// own id, which that page accepts in place of its slug.
func msgMilestoneClosed(actor string, m db.Milestone, sum db.MilestoneCloseSummaryRow) notifyMsg {
	title := strings.TrimSpace(m.Title)
	p := event(evMilestoneClosed, nil)
	p["actor"] = actor
	p["title"] = title
	p["milestone_id"] = m.ID.String()
	p["project_id"] = m.ProjectID.String()
	p["project"] = sum.ProjectName
	p["project_slug"] = sum.ProjectSlug
	p["done"] = sum.DoneCount
	p["total"] = sum.TaskCount
	return notifyMsg{
		text: fmt.Sprintf("%s закрыл(а) этап «%s» — выполнено %d из %d",
			actor, title, sum.DoneCount, sum.TaskCount),
		payload: p,
	}
}

// msgConferenceStarting is «встреча через 5 минут». Raised by the scanner, so it
// has no actor; the title is always inlined (like a due date's: without it the
// line says nothing), and the start rides along for the client to show.
func msgConferenceStarting(title, confID string, at time.Time, minutes int) notifyMsg {
	title = strings.TrimSpace(title)
	p := event(evConfStarting, nil)
	p["title"] = title
	p["conference_id"] = confID
	p["scheduled_at"] = at.UTC().Format(time.RFC3339)
	p["minutes"] = minutes
	return notifyMsg{
		text:    fmt.Sprintf("Через %d мин начнётся встреча «%s»", minutes, title),
		payload: p,
	}
}

// msgSyncConflict reports a change that could not be pushed to an integration
// because both sides edited the same fields (#3048). Provider-neutral: `label`
// names the integration for the sentence, `provider` lets a client pick its
// glyph, and the fields are the machine names the client words itself.
func msgSyncConflict(provider, label string, number *int64, title string, fields []string) notifyMsg {
	p := withCtx(event(evSyncConflict, number), "title", title)
	p["provider"] = provider
	p["label"] = label
	p["fields"] = fields
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		if w, ok := fieldWords[f]; ok {
			words = append(words, w)
		}
	}
	return notifyMsg{
		text: fmt.Sprintf("%s · конфликт синхронизации в #%s (%s): изменено в обоих местах",
			label, taskRef(number), strings.Join(words, ", ")),
		payload: p,
	}
}

// msgDocMention / msgConfMention are mentions outside a task. The place is named
// under `name` (not `title`: the inlined context of a mention is the comment
// itself, as in task_mention), the comment travels as the excerpt and the quote.
func msgDocMention(actor string, doc db.Document, body string) notifyMsg {
	name := strings.TrimSpace(doc.Title)
	p := withCtx(event(evDocMention, nil), "excerpt", body)
	p["actor"] = actor
	p["name"] = name
	p["document_id"] = doc.ID.String()
	p["document_slug"] = doc.Slug
	withQuote(p, body)
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в документе «%s»%s", actor, name, shortCtx(body)),
		payload: p,
	}
}

func msgConfMention(actor string, conf db.Conference, body string) notifyMsg {
	name := strings.TrimSpace(conf.Title)
	p := withCtx(event(evConfMention, nil), "excerpt", body)
	p["actor"] = actor
	p["name"] = name
	p["conference_id"] = conf.ID.String()
	withQuote(p, body)
	return notifyMsg{
		text:    fmt.Sprintf("%s упомянул(а) вас в чате встречи «%s»%s", actor, name, shortCtx(body)),
		payload: p,
	}
}

// notifyMilestoneClosed tells the people the stage's tasks are assigned to that
// it was closed — only on the transition, and only when a person closed it here:
// a stage closed in GitLab arrives through the sync, which reports itself.
func (h *API) notifyMilestoneClosed(c *gin.Context, before, after db.Milestone, wsID uuid.UUID) {
	if before.State == "closed" || after.State != "closed" {
		return
	}
	sum, err := h.q.MilestoneCloseSummary(c, after.ID)
	if err != nil {
		soft(c, "MilestoneCloseSummary", err)
		return
	}
	ids, err := h.q.MilestoneAssignees(c, &after.ID)
	if err != nil {
		soft(c, "MilestoneAssignees", err)
		return
	}
	msg := msgMilestoneClosed(h.actorName(c), after, sum)
	for _, uid := range ids {
		h.notify(c, uid, wsID, nil, kindMilestoneClosed, msg)
	}
}

// notifyMentionsOf sends a "mention" to each listed member once (notify skips
// the author and deliverNotification anyone no longer in the workspace). ids
// are what the client resolved from the chips it drew, exactly as for a task
// comment.
func (h *API) notifyMentionsOf(c *gin.Context, wsID uuid.UUID, ids []uuid.UUID, msg notifyMsg) {
	seen := map[uuid.UUID]bool{}
	for _, uid := range ids {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		h.notify(c, uid, wsID, nil, kindMention, msg)
	}
}

// notifyWritebackConflict tells whoever made the change that it is parked as a
// conflict and waits for their decision — the person who can say which side
// wins. A change with no recorded author (an old row) goes to the task's people
// instead. Best-effort, off the request path.
func (h *API) notifyWritebackConflict(ctx context.Context, w db.GitlabWriteback, wsID uuid.UUID, fields []conflictField) {
	task, err := h.q.GetTask(ctx, w.TaskID)
	if err != nil {
		return
	}
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Field)
	}
	msg := msgSyncConflict("gitlab", "GitLab", task.Number, task.Title, names)
	recipients := []uuid.UUID{}
	if w.ActorUserID != nil {
		recipients = append(recipients, *w.ActorUserID)
	} else {
		recipients = h.dueRecipients(ctx, task)
	}
	for _, uid := range recipients {
		h.deliverNotification(ctx, uid, wsID, &task.ID, nil, kindIntegrationConflict, msg)
	}
}

// scanConferencesSoon raises «встреча через 5 минут» for every planned meeting
// about to start, to everyone invited who has not declined. The stamp is taken
// before the fan-out and guarded on the start time, so a tick that overlaps
// another (the scanner lock aside) cannot ring twice.
func (h *API) scanConferencesSoon(ctx context.Context) {
	confs, err := h.q.ListConferencesStartingSoon(ctx, int32(conferenceSoonLead/time.Minute))
	if err != nil {
		return
	}
	now := time.Now()
	for _, conf := range confs {
		if conf.ScheduledAt == nil {
			continue
		}
		won, err := h.q.MarkConferenceSoonNotified(ctx, db.MarkConferenceSoonNotifiedParams{
			ID: conf.ID, ScheduledAt: conf.ScheduledAt,
		})
		if err != nil || won == 0 {
			continue
		}
		parts, err := h.q.ListConferenceParticipants(ctx, conf.ID)
		if err != nil {
			log.Printf("notify: participants of %s: %v", conf.ID, err)
			continue
		}
		minutes := max(1, int(math.Ceil(conf.ScheduledAt.Sub(now).Minutes())))
		msg := msgConferenceStarting(conf.Title, conf.ID.String(), *conf.ScheduledAt, minutes)
		for _, p := range parts {
			if p.Rsvp == rsvpDeclined {
				continue
			}
			h.deliverNotification(ctx, p.UserID, conf.WorkspaceID, nil, nil, kindConferenceSoon, msg)
		}
	}
}
