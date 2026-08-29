package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"tessera/internal/observability"
)

// checkUploadDir proves at boot that UPLOAD_DIR is actually writable, by doing
// what the upload handlers do: create the directory and put a file in it.
//
// The failure this guards against is #2820 — a named volume whose mount point
// the Docker daemon created as root:root while the container runs as nonroot
// (65532). Nothing in the boot path touches the upload directory, so the stack
// came up green and only broke later, at the moment a user attached an image.
// A probe here turns that into one loud line in the log (and in Sentry) at
// startup.
func checkUploadDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("создать каталог: %w", err)
	}
	// A real write: the directory can exist and still be read-only for us, which
	// is exactly the volume-ownership case.
	f, err := os.CreateTemp(dir, ".preflight-*")
	if err != nil {
		return fmt.Errorf("записать пробный файл: %w", err)
	}
	name := f.Name()
	// Best-effort cleanup; the probe has already answered the question by here.
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// preflightUploadDir runs checkUploadDir and reports a failure loudly — but does
// not stop the process. Attachments being broken is a partial degradation;
// refusing to boot would turn it into a full outage of a self-hosted instance,
// which is worse. The message carries the process uid/gid and a ready-to-paste
// fix, because the operator reading it is looking at a container they cannot
// get a shell into.
func preflightUploadDir(dir string) {
	err := checkUploadDir(dir)
	if err == nil {
		slog.Info("uploads: каталог доступен на запись", "dir", dir)
		return
	}
	abs, absErr := filepath.Abs(dir)
	if absErr != nil {
		abs = dir
	}
	slog.Error("uploads: каталог НЕ доступен на запись — загрузка файлов и вложений будет падать",
		"dir", abs,
		"uid", os.Getuid(),
		"gid", os.Getgid(),
		"err", err,
		"fix", "docker run --rm -v <проект>_backend_uploads:/d busybox chown -R "+
			fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())+" /d",
	)
	observability.CaptureError("uploads-preflight", fmt.Errorf("upload dir %s не доступен на запись: %w", abs, err))
}
