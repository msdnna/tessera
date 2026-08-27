//go:build e2e

// The *production* image, which nothing else covers. docker_test.go builds
// `Dockerfile` — the alpine dev image that runs as root — so the distroless
// nonroot image that actually ships was untested, and #2820 (an uploads volume
// the container cannot write to) reached production through exactly that gap.
//
// Opt-in for the same reason as docker_test.go: set E2E_DOCKER=1, or run
// `make test-e2e-backend-docker`.
//
// Deliberately no HTTP: this asserts through `docker logs` and a second
// container on the same volume, never over a port. What is under test is the
// image/volume/uid interaction, and the API surface is covered by the rest of
// the suite — while host↔container networking is the least portable thing
// available (under Docker Desktop/WSL2 neither --network host nor a published
// port reaches the container from the host).
package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The uid/gid the distroless `nonroot` user resolves to, and the identity the
// backend process runs under in production.
const nonrootUID = "65532"

// TestProdImageOwnsFreshUploadsVolume is the #2820 regression.
//
// Docker seeds a *fresh* named volume from the image at the mount path, owner
// included. When the image carries no /data/uploads, the daemon creates the
// mount point itself as root:root while the container runs as 65532, and every
// upload fails with EACCES. Boot stays green either way, which is why this shows
// up only when a user attaches a file — so the assertions here are the two
// things that actually differ: who owns the directory, and whether the process
// can write into it.
func TestProdImageOwnsFreshUploadsVolume(t *testing.T) {
	if os.Getenv("E2E_DOCKER") != "1" {
		t.Skip("docker tier is opt-in: set E2E_DOCKER=1 (make test-e2e-backend-docker)")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}

	image := "tessera-e2e-prod:" + runID
	container := "tessera-e2e-prod-" + runID
	volume := "tessera-e2e-uploads-" + runID

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancelBuild()

	args := []string{"build", "-f", "Dockerfile.prod", "-t", image}
	for _, v := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if val := os.Getenv(v); val != "" {
			args = append(args, "--build-arg", v+"="+val)
		}
	}
	args = append(args, ".")

	t.Logf("docker build -f Dockerfile.prod %s (cold cache: several minutes)", image)
	build := exec.CommandContext(buildCtx, "docker", args...)
	build.Dir = backendDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, tail(string(out), 40))
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", image).Run() })

	// Fresh volume: ownership is decided at first mount and never revisited, so
	// a reused one would only report what an earlier run left behind.
	if out, err := exec.Command("docker", "volume", "create", volume).CombinedOutput(); err != nil {
		t.Fatalf("docker volume create: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", volume).Run() })

	// ── 1. Whether the process can actually write there ────────────────────
	// The backend probes UPLOAD_DIR at boot (preflightUploadDir) with a real
	// MkdirAll + file write, as the nonroot user, against this very volume — an
	// end-to-end answer that needs no port. The probe runs before the database
	// connection, so this holds whether or not the container reaches Postgres.
	//
	// This has to be the volume's *first* mount: Docker seeds a fresh volume
	// from whichever image mounts it first, so probing the ownership with a
	// helper image beforehand would seed it from the helper and measure nothing.
	run := exec.Command("docker", "run", "-d", "--name", container,
		"-v", volume+":/data/uploads",
		"-e", "DATABASE_URL="+serverDBURL,
		"-e", "JWT_SECRET="+testJWTSecret,
		"-e", "ENCRYPTION_KEY="+testEncKey,
		"-e", "UPLOAD_DIR=/data/uploads",
		image)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", container).Run() })

	logs := awaitLogLine(t, container, "uploads: каталог")
	if strings.Contains(logs, "НЕ доступен на запись") {
		t.Errorf("the container cannot write to a fresh uploads volume (#2820):\n%s", tail(logs, 20))
	}

	// ── 2. Who owns the mount point ────────────────────────────────────────
	// The root cause itself, read straight off the volume now that the image
	// under test has seeded it. Distroless has no shell, so this borrows the
	// builder's base image. Only stdout is read: a pull would otherwise land in
	// the middle of the value.
	owner, err := exec.Command("docker", "run", "--rm", "-v", volume+":/d",
		"golang:1.25-alpine", "stat", "-c", "%u:%g", "/d").Output()
	if err != nil {
		t.Fatalf("stat the uploads volume: %v", err)
	}
	if got := strings.TrimSpace(string(owner)); got != nonrootUID+":"+nonrootUID {
		t.Errorf("backend_uploads volume is owned by %s, want %s:%s — "+
			"Dockerfile.prod is not seeding /data/uploads for nonroot (#2820)",
			got, nonrootUID, nonrootUID)
	}
}

// awaitLogLine polls `docker logs` until it contains want, and returns the log.
// Failing with the container's own output beats a bare timeout.
func awaitLogLine(t *testing.T, container, want string) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "logs", container).CombinedOutput()
		if err == nil {
			last = string(out)
			if strings.Contains(last, want) {
				return last
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container never logged %q:\n%s", want, tail(last, 40))
	return ""
}
