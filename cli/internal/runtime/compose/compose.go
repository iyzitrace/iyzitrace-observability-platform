// Package compose implements runtime.Runtime by shelling out to `docker compose`.
//
// We intentionally do not use the docker compose Go module — it has a heavy
// dep tree and the CLI is the supported user-facing interface anyway. v2 of
// the runtime will swap to the Docker SDK directly.
package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/runtime"
)

type Backend struct {
	ComposeFile string // path to docker-compose.yml
	EnvFile     string // optional path to .env (passed via --env-file)
	WorkingDir  string // cwd for `docker compose` (so relative volume paths resolve)
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
}

func New(composeFile string) *Backend {
	return &Backend{
		ComposeFile: composeFile,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
	}
}

func (b *Backend) base(args ...string) []string {
	out := []string{"compose", "-f", b.ComposeFile}
	if b.EnvFile != "" {
		out = append(out, "--env-file", b.EnvFile)
	}
	return append(out, args...)
}

func (b *Backend) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", b.base(args...)...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = b.Stdout, b.Stderr, b.Stdin
	if b.WorkingDir != "" {
		cmd.Dir = b.WorkingDir
	}
	return cmd.Run()
}

func (b *Backend) Up(ctx context.Context, services []string, opts runtime.UpOptions) error {
	args := []string{"up"}
	if opts.Detach {
		args = append(args, "-d")
	}
	args = append(args, "--remove-orphans")
	args = append(args, services...)
	return b.run(ctx, args...)
}

func (b *Backend) Down(ctx context.Context, opts runtime.DownOptions) error {
	args := []string{"down", "--remove-orphans"}
	if opts.RemoveVolumes {
		args = append(args, "--volumes")
	}
	return b.run(ctx, args...)
}

func (b *Backend) Restart(ctx context.Context, services []string) error {
	args := append([]string{"restart"}, services...)
	return b.run(ctx, args...)
}

func (b *Backend) Logs(ctx context.Context, service string, opts runtime.LogsOptions) error {
	args := []string{"logs"}
	if opts.Follow {
		args = append(args, "-f")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(opts.Tail))
	}
	if service != "" {
		args = append(args, service)
	}
	return b.run(ctx, args...)
}

// PS returns the container list parsed from `docker compose ps --format json`.
// We capture stdout instead of streaming.
func (b *Backend) PS(ctx context.Context) ([]runtime.Container, error) {
	cmd := exec.CommandContext(ctx, "docker", b.base("ps", "--format", "json", "--all")...)
	cmd.Stderr = b.Stderr
	if b.WorkingDir != "" {
		cmd.Dir = b.WorkingDir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// docker compose ps emits NDJSON (one JSON object per line) on newer versions
	// and a single JSON array on older ones. Handle both.
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	var entries []composeEntry
	if trimmed[0] == '[' {
		if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
			return nil, fmt.Errorf("parse compose ps json array: %w", err)
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var e composeEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				return nil, fmt.Errorf("parse compose ps line: %w", err)
			}
			entries = append(entries, e)
		}
	}

	out2 := make([]runtime.Container, 0, len(entries))
	for _, e := range entries {
		out2 = append(out2, runtime.Container{
			Name:   firstNonEmpty(e.Name, e.Service),
			Image:  e.Image,
			State:  e.State,
			Status: e.Status,
		})
	}
	return out2, nil
}

type composeEntry struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	Image   string `json:"Image"`
	State   string `json:"State"`
	Status  string `json:"Status"`
}

func firstNonEmpty(xs ...string) string {
	for _, s := range xs {
		if s != "" {
			return s
		}
	}
	return ""
}

// EnsureAvailable verifies the docker compose CLI is reachable.
func EnsureAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "version", "--short")
	if err := cmd.Run(); err != nil {
		return errors.New("docker compose not available; install Docker Engine + Compose v2")
	}
	return nil
}
