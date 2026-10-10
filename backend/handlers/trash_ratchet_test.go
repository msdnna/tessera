package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The trash ratchet (migration 0086, #3001).
//
// A task or a document in the trash is gone for every reader, and nothing in the
// schema enforces that: each query that reads `tasks` or `documents` has to say
// `deleted_at IS NULL` for every alias it reads the table under. One query that
// forgets is a deleted task back on a board, in search, in «Мои задачи» or in a
// notification. This test reads db/queries/*.sql and fails on a query that
// reads one of the two tables under an alias whose deleted_at it never mentions.
//
// Some queries must see the trash on purpose — the trash's own list, restore and
// purge, the upload sweeps of a container delete, the cross-workspace cleanups
// of a project transfer. They are listed in trashBlind with the reason. The list
// is a ratchet: a new entry needs a reason a reviewer can check, and an entry
// that no longer reads the table blind fails the test until it is removed.

// trashBlind maps "file.sql:QueryName" to why the query reads the trash too.
var trashBlind = map[string]string{
	// Containers: a board's or column's tasks travel with it whatever their state.
	"boards.sql:DeletedBoardTaskIDs": "board purge sweeps every task's uploads, trashed ones included",
	"boards.sql:RecordColumnMoves":   "a deleted column hands ALL its tasks over, so a trashed task never sits in a purged column",
	"boards.sql:MoveColumnTasks":     "same as RecordColumnMoves",
	// Project transfer: cross-workspace cleanups must cover the trash, which travels too.
	"conferences.sql:ClearConferenceTasksAcrossWorkspaces":        "transfer cleanup covers trashed tasks",
	"document_task_links.sql:DeleteDocumentLinksAcrossWorkspaces": "transfer cleanup covers trashed tasks and documents",
	"task_relations.sql:DeleteRelationsAcrossWorkspaces":          "transfer cleanup covers trashed tasks",
	"tags.sql:DeleteTaskTagsAcrossWorkspaces":                     "transfer cleanup covers trashed tasks",
	"tags.sql:StripNonMemberAssignees":                            "transfer cleanup covers trashed tasks",
	"tags.sql:RestoreTaskTag":                                     "a restored tag goes back on a trashed task too, which may come back later",
	"projects.sql:CountProjectTasks":                              "a trashed task keeps its number, so the transfer reserves one for it",
	"projects.sql:RenumberProjectTasks":                           "same as CountProjectTasks",
	"document_approvals.sql:CancelApprovalsAcrossWorkspaces":      "transfer cleanup covers trashed documents",
	"documents.sql:DetachDocumentParentsAcrossWorkspaces":         "transfer cleanup covers trashed documents",
	"documents.sql:DocumentIDsInProject":                          "transfer drops the rooms and moves every document of the project",
	"documents.sql:ProjectDocumentSlugs":                          "a trashed document keeps its slug (unique per workspace)",
	"documents.sql:DocumentIDsInWorkspace":                        "workspace delete sweeps every document's images",
	"documents.sql:DocumentSlugExists":                            "the slug index covers trashed documents",
	"documents.sql:DocumentsMissingSlug":                          "slug backfill at startup, state-blind",
	"documents.sql:CountDocumentAssetRefs":                        "a trashed body still embeds its images until it is purged",
	"document_approvals.sql:LockDocumentApprovalSlot":             "row lock by id; the caller has loaded a live document",
	"documents.sql:DocumentAncestorIDs":                           "cycle guard on parent links must see every row",
	"documents.sql:DeleteDocumentSubtree":                         "the hard delete behind «Удалить навсегда»",
	"documents.sql:GetDocumentAny":                                "the trash's own restore / purge lookup",
	"documents.sql:PurgeDeletedDocuments":                         "trash_purge: the subtree of an expired root goes whole",
	"gitlab.sql:ListGitlabChildGlobalIDs":                         "GitLab-side hierarchy: a trashed child is still a child in GitLab",
	"gitlab.sql:CountGitlabChildLinks":                            "same as ListGitlabChildGlobalIDs",
	"milestones.sql:SubtreeMilestonesOffProject":                  "journals the milestone of every task MoveSubtreeToBoard carries, trash included",
	"milestones.sql:MilestoneMembershipEvents":                    "journal matching only; MilestoneEventTasks flags trashed tasks as gone",
	"tasks.sql:GetTaskAny":                                        "the trash's own lookups (restore, purge, GitLab sync, delivery reach)",
	"tasks.sql:MoveSubtreeToBoard":                                "a subtree stays on one board, its trashed members included",
	"tasks.sql:ReopenSubtreeLeavingDone":                          "runs with MoveSubtreeToBoard over the same subtree",
	"tasks.sql:TaskAncestorIDs":                                   "cycle guard on parent links must see every row",
	"tasks.sql:DeleteTask":                                        "the hard delete behind «Удалить навсегда»",
	"tasks.sql:PurgeDeletedTasks":                                 "trash_purge: the subtree of an expired root goes whole",
	"tasks.sql:TaskIDsInColumn":                                   "container delete sweeps every task's uploads",
	"tasks.sql:TaskIDsOnBoard":                                    "container delete sweeps every task's uploads",
	"tasks.sql:TaskIDsInProject":                                  "container delete sweeps every task's uploads",
	"tasks.sql:TaskIDsInWorkspace":                                "container delete sweeps every task's uploads",
}

var (
	queryName = regexp.MustCompile(`(?m)^-- name: (\w+)`)
	// FROM/JOIN/USING tasks [AS] alias, and the comma form of a FROM list.
	tableRef = regexp.MustCompile(`(?i)(?:\b(?:from|join|using)\s+|,\s*)(tasks|documents)\b(?:\s+(?:as\s+)?([a-z_][a-z0-9_]*))?`)
	sqlWords = map[string]bool{
		"where": true, "on": true, "join": true, "left": true, "right": true, "inner": true,
		"full": true, "cross": true, "natural": true, "order": true, "group": true, "set": true,
		"limit": true, "returning": true, "union": true, "using": true, "for": true, "having": true,
		"window": true, "offset": true, "except": true, "intersect": true, "lateral": true,
	}
)

func stripSQLComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// mentionsDeleted reports whether body filters alias on deleted_at. A bare
// table name (no alias) also accepts an unqualified deleted_at.
func mentionsDeleted(body, table, alias string) bool {
	if strings.Contains(body, alias+".deleted_at") {
		return true
	}
	if alias != table {
		return false
	}
	for i := 0; ; {
		j := strings.Index(body[i:], "deleted_at")
		if j < 0 {
			return false
		}
		j += i
		if j == 0 || body[j-1] != '.' {
			return true
		}
		i = j + len("deleted_at")
	}
}

func TestQueriesFilterTheTrash(t *testing.T) {
	files, err := filepath.Glob("../db/queries/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	blindSeen := map[string]bool{}
	var leaks []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		locs := queryName.FindAllStringSubmatchIndex(src, -1)
		for i, loc := range locs {
			name := src[loc[2]:loc[3]]
			end := len(src)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			body := strings.ToLower(stripSQLComments(src[loc[1]:end]))
			key := filepath.Base(f) + ":" + name
			blind := false
			for _, m := range tableRef.FindAllStringSubmatch(body, -1) {
				table, alias := m[1], m[2]
				if alias == "" || sqlWords[alias] {
					alias = table
				}
				if !mentionsDeleted(body, table, alias) {
					blind = true
					if _, ok := trashBlind[key]; !ok {
						leaks = append(leaks, key+" reads "+table+" as "+alias+" without "+alias+".deleted_at")
					}
				}
			}
			if blind {
				blindSeen[key] = true
			}
		}
	}
	sort.Strings(leaks)
	for _, l := range leaks {
		t.Errorf("trash leak: %s — filter it with deleted_at IS NULL, or list the query in trashBlind with the reason", l)
	}
	for key := range trashBlind {
		if !blindSeen[key] {
			t.Errorf("trashBlind entry %s no longer reads the trash — remove it (the list only shrinks)", key)
		}
	}
}
