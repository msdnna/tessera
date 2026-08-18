package gitlab

import (
	"context"
	"strconv"
	"strings"
)

// GitLab's issue hierarchy (a "grouped" issue with child tasks under it) only exists
// through the work-items API. The REST /issues/:iid/links endpoint is NOT it — that
// builds relates_to/blocks relations, which are a different thing entirely and show up
// elsewhere in the UI.
//
// Creating a child is deliberately split into two operations instead of the single
// `workItemCreate` mutation the obvious reading suggests:
//
//  1. create the issue with issue_type=task over REST (see Client.CreateIssue), then
//  2. attach it to its parent with `workItemUpdate` + hierarchyWidget (below).
//
// The reason is that `WorkItemCreateInput` is the one shape that drifts across GitLab
// versions (projectPath vs namespacePath, description as a field vs a widget), so
// writing that mutation without introspecting the target instance is guesswork. Step 2
// has no such fields — just an id and a parent id — and step 1 reuses the whole
// existing create path (labels, assignees, due date, milestone, the gitlab_links row).
//
// Every call reports `supported` separately from `err`, the same contract as
// LinkedItems: an instance whose schema has no hierarchy widget is a fact to degrade
// on, not a failure to retry.

// workItemGIDQuery reads a work item's own global id by iid. The gid is READ, never
// constructed: "gid://gitlab/WorkItem/<n>" and "gid://gitlab/Issue/<n>" carry the same
// trailing number today, but that is GitLab's internal business and not a documented
// identity — the hierarchy mutation rejects an Issue gid.
const workItemGIDQuery = `
query($path: ID!, $iid: String!) {
  project(fullPath: $path) {
    workItems(iid: $iid) {
      nodes {
        id
        widgets {
          ... on WorkItemWidgetHierarchy {
            parent { id }
          }
        }
      }
    }
  }
}`

// WorkItem is the hierarchy-relevant view of one issue: its own work-item gid and its
// parent's (empty when top-level).
type WorkItem struct {
	GID       string // gid://gitlab/WorkItem/<n>
	ParentGID string // gid://gitlab/WorkItem/<n>, empty when it has no parent
}

// WorkItemByIID resolves an issue's work-item gid and current parent gid.
//
// supported=false means this instance exposes no hierarchy widget for the item, so the
// caller must skip the hierarchy step rather than retry it. A missing project or an
// unknown iid is (WorkItem{}, false, nil) — nothing to attach to, and nothing broken.
func (c *Client) WorkItemByIID(ctx context.Context, projectPath string, iid int64) (WorkItem, bool, error) {
	var data struct {
		Project *struct {
			WorkItems struct {
				Nodes []struct {
					ID      string `json:"id"`
					Widgets []struct {
						Parent *struct {
							ID string `json:"id"`
						} `json:"parent"`
					} `json:"widgets"`
				} `json:"nodes"`
			} `json:"workItems"`
		} `json:"project"`
	}
	vars := map[string]any{"path": projectPath, "iid": strconv.FormatInt(iid, 10)}
	if err := c.do(ctx, workItemGIDQuery, vars, &data); err != nil {
		return WorkItem{}, false, err
	}
	if data.Project == nil || len(data.Project.WorkItems.Nodes) == 0 {
		return WorkItem{}, false, nil
	}
	node := data.Project.WorkItems.Nodes[0]
	if node.ID == "" {
		return WorkItem{}, false, nil
	}
	wi := WorkItem{GID: node.ID}
	for _, w := range node.Widgets {
		if w.Parent != nil && w.Parent.ID != "" {
			wi.ParentGID = w.Parent.ID
			break
		}
	}
	return wi, true, nil
}

// setParentMutation attaches (parentId set) or detaches (parentId null) a work item.
// Unlike workItemCreate, its input has no version-dependent fields — id and
// hierarchyWidget.parentId are all it takes.
const setParentMutation = `
mutation($id: WorkItemID!, $parentId: WorkItemID) {
  workItemUpdate(input: {id: $id, hierarchyWidget: {parentId: $parentId}}) {
    errors
    workItem {
      id
      widgets {
        ... on WorkItemWidgetHierarchy {
          parent { id }
        }
      }
    }
  }
}`

// SetWorkItemParent makes childGID a child of parentGID in GitLab's hierarchy. Pass an
// empty parentGID to detach the child back to top-level.
//
// supported=false covers the two ways an instance can decline structurally — no
// hierarchy widget in the schema, or this parent/child type pair not allowed — as
// opposed to a transport error, which comes back as err. The caller keeps the child
// issue either way: an unattached subtask is a degraded result, a lost one is a bug.
func (c *Client) SetWorkItemParent(ctx context.Context, childGID, parentGID string) (bool, error) {
	if childGID == "" {
		return false, nil
	}
	var data struct {
		WorkItemUpdate *struct {
			Errors   []string `json:"errors"`
			WorkItem *struct {
				ID      string `json:"id"`
				Widgets []struct {
					Parent *struct {
						ID string `json:"id"`
					} `json:"parent"`
				} `json:"widgets"`
			} `json:"workItem"`
		} `json:"workItemUpdate"`
	}
	vars := map[string]any{"id": childGID, "parentId": nil}
	if parentGID != "" {
		vars["parentId"] = parentGID
	}
	if err := c.do(ctx, setParentMutation, vars, &data); err != nil {
		// A schema without the hierarchy widget fails at validation, before any state
		// changed — an unsupported instance, not a transient fault to retry forever.
		if isSchemaError(err) {
			return false, nil
		}
		return false, err
	}
	if data.WorkItemUpdate == nil {
		return false, nil
	}
	if len(data.WorkItemUpdate.Errors) > 0 {
		return false, &APIError{Status: 422, Body: strings.Join(data.WorkItemUpdate.Errors, "; ")}
	}
	return true, nil
}

// isSchemaError reports whether a GraphQL error is the instance rejecting the query
// shape (unknown field/argument/type) rather than failing to run it. GitLab returns
// these as plain validation messages, so matching text is the only signal available —
// and getting it wrong is safe in one direction: a mis-classified error degrades to
// "hierarchy unsupported", which still keeps the child issue.
func isSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "graphql") {
		return false
	}
	for _, marker := range []string{
		"doesn't exist on type",
		"does not exist on type",
		"no field",
		"undefined field",
		"argument",
		"isn't a defined input type",
		"is not a defined input type",
		"unknown type",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
