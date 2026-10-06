package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// File attachment tools. Inline images in comments go through
// tessera_add_comment's image_paths; these two handle real file attachments.

// maxAttachmentBytes mirrors the backend's per-file limit — checking it here
// turns a 25 MiB upload into a clear message instead of a 413 after the wait.
const maxAttachmentBytes = 25 << 20

// registerAttachments wires the attachment tools onto the server.
func registerAttachments(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_upload_attachment",
		Description: "Attach local files to a task (logs, dumps, archives, screenshots you want kept as files). " +
			"Max 25 MiB each. To show an image inline in a comment use tessera_add_comment's image_paths instead.",
	}, uploadAttachment(c))

	mcp.AddTool(s, &mcp.Tool{
		Name: "tessera_download_attachment",
		Description: "Download a task's attachments to a local directory so you can open them. Name them by " +
			"attachment_id or filename, or omit both to fetch all of the task's attachments. Returns the paths written.",
	}, downloadAttachment(c))
}

// ── upload_attachment ─────────────────────────────────────────────────────────

type uploadAttachmentInput struct {
	TaskID      string   `json:"task_id,omitempty" jsonschema:"the task (or subtask) UUID"`
	WorkspaceID string   `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number      int64    `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	FilePaths   []string `json:"file_paths" jsonschema:"local file paths to upload (the MCP server process must be able to read them)"`
}

type attachmentOut struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Path     string `json:"path,omitempty"` // set by download, not upload
}

type uploadAttachmentOut struct {
	TaskID      string          `json:"task_id"`
	Attachments []attachmentOut `json:"attachments"`
}

func uploadAttachment(c *client.Client) mcp.ToolHandlerFor[uploadAttachmentInput, uploadAttachmentOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in uploadAttachmentInput) (*mcp.CallToolResult, uploadAttachmentOut, error) {
		if len(in.FilePaths) == 0 {
			return nil, uploadAttachmentOut{}, fmt.Errorf("file_paths is required")
		}
		taskID, err := resolveTaskID(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, uploadAttachmentOut{}, err
		}

		out := uploadAttachmentOut{TaskID: taskID}
		for _, p := range in.FilePaths {
			info, sErr := os.Stat(p)
			if sErr != nil {
				return nil, uploadAttachmentOut{}, fmt.Errorf("read %q: %w", p, sErr)
			}
			if info.Size() > maxAttachmentBytes {
				return nil, uploadAttachmentOut{}, fmt.Errorf("%q is %d bytes; the backend caps attachments at 25 MiB", p, info.Size())
			}
			data, rErr := os.ReadFile(p)
			if rErr != nil {
				return nil, uploadAttachmentOut{}, fmt.Errorf("read %q: %w", p, rErr)
			}
			name := filepath.Base(p)
			att, uErr := c.UploadAttachment(ctx, taskID, name, attachmentMIME(name), data)
			if uErr != nil {
				return nil, uploadAttachmentOut{}, fmt.Errorf("upload %q: %w", p, uErr)
			}
			out.Attachments = append(out.Attachments, attachmentOut{ID: att.ID, Filename: att.Filename, Size: att.Size})
		}
		return nil, out, nil
	}
}

// attachmentMIME guesses a content type from the filename; "" lets the backend
// record whatever the multipart part carries.
func attachmentMIME(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".txt", ".log", ".md":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// ── download_attachment ───────────────────────────────────────────────────────

type downloadAttachmentInput struct {
	TaskID       string `json:"task_id,omitempty" jsonschema:"the task UUID whose attachments to download"`
	WorkspaceID  string `json:"workspace_id,omitempty" jsonschema:"workspace UUID (with number) to resolve the task"`
	Number       int64  `json:"number,omitempty" jsonschema:"the per-workspace task number, e.g. 252"`
	AttachmentID string `json:"attachment_id,omitempty" jsonschema:"download just this attachment (from tessera_get_task's attachments)"`
	Filename     string `json:"filename,omitempty" jsonschema:"download the task attachment with this filename"`
	DestDir      string `json:"dest_dir,omitempty" jsonschema:"directory to write into; defaults to a temp dir"`
}

type downloadAttachmentOut struct {
	DestDir     string          `json:"dest_dir"`
	Attachments []attachmentOut `json:"attachments"`
}

func downloadAttachment(c *client.Client) mcp.ToolHandlerFor[downloadAttachmentInput, downloadAttachmentOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in downloadAttachmentInput) (*mcp.CallToolResult, downloadAttachmentOut, error) {
		d, err := resolveTaskDetail(ctx, c, in.TaskID, in.WorkspaceID, in.Number)
		if err != nil {
			return nil, downloadAttachmentOut{}, err
		}
		all, err := c.ListAttachments(ctx, d.ID)
		if err != nil {
			return nil, downloadAttachmentOut{}, err
		}
		wanted, err := pickAttachments(all, in.AttachmentID, in.Filename)
		if err != nil {
			return nil, downloadAttachmentOut{}, err
		}

		destDir := in.DestDir
		if destDir == "" {
			destDir = filepath.Join(os.TempDir(), "tessera-attachments", d.ID)
		}
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return nil, downloadAttachmentOut{}, err
		}

		out := downloadAttachmentOut{DestDir: destDir}
		taken := map[string]bool{}
		for _, att := range wanted {
			// The filename comes from whoever uploaded it: Base() strips any
			// directory trickery so the write can't escape dest_dir.
			name := filepath.Base(att.Filename)
			if name == "." || name == string(filepath.Separator) || name == ".." {
				name = att.ID
			}
			name = uniqueName(taken, name)
			path := filepath.Join(destDir, name)
			f, cErr := os.Create(path)
			if cErr != nil {
				return nil, downloadAttachmentOut{}, cErr
			}
			n, dErr := c.DownloadAttachment(ctx, att.ID, f)
			closeErr := f.Close()
			if dErr != nil {
				return nil, downloadAttachmentOut{}, fmt.Errorf("download %q: %w", att.Filename, dErr)
			}
			if closeErr != nil {
				return nil, downloadAttachmentOut{}, closeErr
			}
			out.Attachments = append(out.Attachments, attachmentOut{ID: att.ID, Filename: att.Filename, Size: n, Path: path})
		}
		return nil, out, nil
	}
}

// uniqueName keeps one call's writes from landing on top of each other.
//
// Nothing makes an attachment's filename unique on a task: task_attachments has
// no unique index over (task_id, filename), and UploadAttachment stores
// filepath.Base of whatever was picked — so two "screenshot.png" (a re-upload
// after a correction, the same log picked off two machines) are two ordinary
// rows. Joining dest_dir with the bare name gave both of them one path, and
// os.Create truncates: the file left on disk was the last one listed
// (ListTaskAttachments orders by created_at, so the newest), while the reply
// still reported every attachment — same path, different ids, different sizes.
// A reader that opened the first path got the last one's bytes with nothing
// saying so, and the size beside it belonged to a file that no longer existed
// anywhere (#2932).
//
// The suffix is per call, not per directory: downloading a task twice into the
// same dest_dir is meant to refresh it in place, so a file must keep the name it
// had the first time instead of accumulating -2, -3 on every run. Keyed
// case-insensitively because the server this runs on may sit on a filesystem
// that is (macOS), where "Log.txt" and "log.txt" are also one file.
func uniqueName(taken map[string]bool, name string) string {
	key := strings.ToLower(name)
	if !taken[key] {
		taken[key] = true
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		// A dotfile (".gitignore"): Ext reports the whole name, and "-2.gitignore"
		// would rename the file rather than number it.
		stem, ext = name, ""
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if k := strings.ToLower(cand); !taken[k] {
			taken[k] = true
			return cand
		}
	}
}

// pickAttachments narrows a task's attachments to the requested ones, or returns
// them all when neither id nor filename was given.
//
// An id names exactly one row, so that match returns alone. A filename does not
// (see uniqueName), and answering with the first row was the wrong half of the
// ambiguity: the list is ordered by created_at, so "give me screenshot.png"
// after a corrected re-upload handed back the version that was replaced. Every
// match comes back instead — the caller asked for a file by name and gets each
// file that has it, one path per row, which is the same shape the no-selector
// branch already returns (#2932).
func pickAttachments(all []model.Attachment, id, filename string) ([]model.Attachment, error) {
	if len(all) == 0 {
		return nil, fmt.Errorf("this task has no attachments")
	}
	if id == "" && filename == "" {
		return all, nil
	}
	var names []string
	var matched []model.Attachment
	for _, att := range all {
		names = append(names, att.Filename)
		if id != "" {
			if att.ID == id {
				return []model.Attachment{att}, nil
			}
			continue
		}
		if strings.EqualFold(att.Filename, filename) {
			matched = append(matched, att)
		}
	}
	if len(matched) > 0 {
		return matched, nil
	}
	ref := id
	if ref == "" {
		ref = filename
	}
	return nil, fmt.Errorf("no attachment %q on this task; available: %s", ref, strings.Join(names, ", "))
}
