//go:build e2e

// The release artifact itself. Everything else in this suite runs a binary built
// on the host; this builds the actual image and starts it, which is the only way
// to catch a broken Dockerfile stage, a missing runtime package or an entrypoint
// that no longer boots. Nothing else in the repo touches the image at all.
//
// Opt-in: a docker build downloads a Go toolchain and the module graph, which is
// minutes on a cold cache and hostile on a metered/filtered network. Set
// E2E_DOCKER=1 (or run `make test-e2e-backend-docker`) to include it.
//
// Linux-only by construction: the container joins the host network so it can
// reach the Postgres these tests already use.
package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDockerImageBootsAndServes(t *testing.T) {
	if os.Getenv("E2E_DOCKER") != "1" {
		t.Skip("docker tier is opt-in: set E2E_DOCKER=1 (make test-e2e-backend-docker)")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}

	image := "tessera-e2e:" + runID
	container := "tessera-e2e-" + runID

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancelBuild()

	args := []string{"build", "-f", "Dockerfile", "-t", image}
	// The compose build passes these through too; a box behind a proxy cannot
	// run `go mod download` inside the builder stage without them.
	for _, v := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		if val := os.Getenv(v); val != "" {
			args = append(args, "--build-arg", v+"="+val)
		}
	}
	args = append(args, ".")

	t.Logf("docker build %s (cold cache: several minutes)", image)
	build := exec.CommandContext(buildCtx, "docker", args...)
	build.Dir = backendDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, tail(string(out), 40))
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rmi", "-f", image).Run()
	})

	port := freePort(t)
	run := exec.Command("docker", "run", "-d", "--name", container,
		// Host network: the database under test lives on the host, and this
		// keeps the DSN identical to the one every other test uses.
		"--network", "host",
		"-e", "DATABASE_URL="+serverDBURL,
		"-e", "PORT="+port,
		"-e", "JWT_SECRET="+testJWTSecret,
		"-e", "ENCRYPTION_KEY="+testEncKey,
		"-e", "UPLOAD_DIR=/data/uploads",
		image)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if logs, err := exec.Command("docker", "logs", container).CombinedOutput(); err == nil && t.Failed() {
			t.Logf("container logs:\n%s", tail(string(logs), 40))
		}
		_ = exec.Command("docker", "rm", "-f", container).Run()
	})

	base := "http://127.0.0.1:" + port
	awaitContainerHealthy(t, container, base)

	// Readiness, not just liveness: it pings the database, so a green answer
	// proves the container's own connection to Postgres works.
	res, err := http.Get(base + "/api/health/ready") //nolint:noctx // short-lived probe
	if err != nil {
		t.Fatalf("GET /api/health/ready: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/api/health/ready = %d\n%s", res.StatusCode, body)
	}

	// The documented operator path: `docker compose exec backend /app/migrate`.
	// It is a second binary in the same image and can rot independently.
	mig, err := exec.Command("docker", "exec", container, "/app/migrate", "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("/app/migrate -version inside the image: %v\n%s", err, mig)
	}
	if !strings.Contains(string(mig), "schema version:") {
		t.Errorf("/app/migrate did not report a version:\n%s", mig)
	}
}

// awaitContainerHealthy polls the published port, failing with the container's
// own logs rather than a bare timeout.
func awaitContainerHealthy(t *testing.T, container, base string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/api/health") //nolint:noctx // bounded by the loop deadline
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		if state, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container).Output(); err == nil {
			if strings.TrimSpace(string(state)) == "false" {
				logs, _ := exec.Command("docker", "logs", container).CombinedOutput()
				t.Fatalf("container exited during boot:\n%s", tail(string(logs), 40))
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	logs, _ := exec.Command("docker", "logs", container).CombinedOutput()
	t.Fatalf("container never answered /api/health:\n%s", tail(string(logs), 40))
}

// tail keeps the last n lines of a command's output — a failed docker build
// otherwise buries the actual error under the whole transcript.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return fmt.Sprintf("… (%d earlier lines omitted)\n%s", len(lines)-n, strings.Join(lines[len(lines)-n:], "\n"))
}
