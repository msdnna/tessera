package gitlab

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// LinkedItem is one GitLab issue-link edge as seen from a source issue: the other
// endpoint plus the direction. Provider-shaped on purpose — the translation into
// Tessera's neutral relation vocabulary happens in RelationKind, and nothing below
// this file's API leaks GitLab's link_type into the core.
type LinkedItem struct {
	LinkType    string // relates_to | blocks | is_blocked_by
	ProjectPath string // full path of the linked issue's project ("group/project")
	IID         int64  // the linked issue's iid within that project
	LinkID      string // the remote link's id (GraphQL gid or REST numeric id)
	WebURL      string
}

// RelationKind maps a GitLab link_type onto a Tessera relation kind. GitLab's three
// types line up exactly with three of Tessera's own, so no new kinds are needed.
// Reports false for anything unrecognised, so an unknown type is skipped rather
// than stored as a bogus relation.
func RelationKind(linkType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(linkType)) {
	case "relates_to":
		return "relates", true
	case "blocks":
		return "blocks", true
	case "is_blocked_by":
		return "blocked_by", true
	default:
		return "", false
	}
}

const linkedItemsQuery = `
query($path: ID!, $iids: [String!]) {
  project(fullPath: $path) {
    workItems(iids: $iids) {
      nodes {
        iid
        widgets {
          ... on WorkItemWidgetLinkedItems {
            linkedItems {
              nodes {
                linkType
                linkId
                workItem {
                  iid
                  webUrl
                  namespace { fullPath }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// LinkedItems fetches the linked-items widget for a batch of issue iids in one
// round-trip, keyed by source iid.
//
// The second return value reports whether the LinkedItems widget was present at
// all: GitLab returns every widget of a work item type (with an empty node list
// when there are no links), so a missing widget means this instance's schema
// predates it and the caller must fall back to IssueLinksREST. A genuine
// "no links anywhere" answer still reports supported=true and an empty map.
func (c *Client) LinkedItems(ctx context.Context, projectPath string, iids []string) (map[int64][]LinkedItem, bool, error) {
	if len(iids) == 0 {
		return map[int64][]LinkedItem{}, true, nil
	}
	var data struct {
		Project *struct {
			WorkItems struct {
				Nodes []struct {
					IID     string `json:"iid"`
					Widgets []struct {
						LinkedItems *struct {
							Nodes []struct {
								LinkType string `json:"linkType"`
								LinkID   string `json:"linkId"`
								WorkItem *struct {
									IID       string `json:"iid"`
									WebURL    string `json:"webUrl"`
									Namespace *struct {
										FullPath string `json:"fullPath"`
									} `json:"namespace"`
								} `json:"workItem"`
							} `json:"nodes"`
						} `json:"linkedItems"`
					} `json:"widgets"`
				} `json:"nodes"`
			} `json:"workItems"`
		} `json:"project"`
	}
	if err := c.do(ctx, linkedItemsQuery, map[string]any{"path": projectPath, "iids": iids}, &data); err != nil {
		return nil, false, err
	}
	out := map[int64][]LinkedItem{}
	if data.Project == nil {
		return out, false, nil
	}
	supported := false
	for _, node := range data.Project.WorkItems.Nodes {
		src, err := strconv.ParseInt(node.IID, 10, 64)
		if err != nil {
			continue
		}
		for _, w := range node.Widgets {
			if w.LinkedItems == nil {
				continue
			}
			supported = true
			for _, n := range w.LinkedItems.Nodes {
				if n.WorkItem == nil {
					continue
				}
				iid, perr := strconv.ParseInt(n.WorkItem.IID, 10, 64)
				if perr != nil {
					continue
				}
				path := projectPath
				if n.WorkItem.Namespace != nil && n.WorkItem.Namespace.FullPath != "" {
					path = n.WorkItem.Namespace.FullPath
				}
				out[src] = append(out[src], LinkedItem{
					LinkType:    n.LinkType,
					ProjectPath: path,
					IID:         iid,
					LinkID:      n.LinkID,
					WebURL:      n.WorkItem.WebURL,
				})
			}
		}
	}
	return out, supported, nil
}

// IssueLinksREST is the fallback for instances whose GraphQL schema has no
// LinkedItems widget: GET /projects/:esc/issues/:iid/links, which every supported
// GitLab has. Returns the links of a single issue.
func (c *Client) IssueLinksREST(ctx context.Context, projectPath string, iid int64) ([]LinkedItem, error) {
	body, err := c.restGet(ctx, issuePath(projectPath, iid)+"/links")
	if err != nil {
		return nil, err
	}
	return parseIssueLinksREST(body, projectPath)
}

// parseIssueLinksREST decodes the REST link list. srcPath is the project the links
// were read from — the fallback for a reference we cannot qualify.
func parseIssueLinksREST(body []byte, srcPath string) ([]LinkedItem, error) {
	var raw []struct {
		IID        int64  `json:"iid"`
		LinkID     int64  `json:"issue_link_id"`
		LinkType   string `json:"link_type"`
		WebURL     string `json:"web_url"`
		References *struct {
			Full string `json:"full"`
		} `json:"references"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]LinkedItem, 0, len(raw))
	for _, r := range raw {
		if r.IID == 0 {
			continue
		}
		path := srcPath
		if r.References != nil {
			if p, _, ok := strings.Cut(r.References.Full, "#"); ok && p != "" {
				path = p
			}
		}
		linkID := ""
		if r.LinkID != 0 {
			linkID = strconv.FormatInt(r.LinkID, 10)
		}
		out = append(out, LinkedItem{
			LinkType:    r.LinkType,
			ProjectPath: path,
			IID:         r.IID,
			LinkID:      linkID,
			WebURL:      r.WebURL,
		})
	}
	return out, nil
}
