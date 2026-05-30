// Package runtime is the interface every backend implements. v1 ships a
// docker-compose wrapper. v2 will add a Docker SDK backend (and possibly
// systemd / kubernetes) without changing the CLI surface.
package runtime

import "context"

type Runtime interface {
	Up(ctx context.Context, services []string, opts UpOptions) error
	Down(ctx context.Context, opts DownOptions) error
	Restart(ctx context.Context, services []string) error
	Logs(ctx context.Context, service string, opts LogsOptions) error
	PS(ctx context.Context) ([]Container, error)
}

type UpOptions struct {
	Detach bool
}

type DownOptions struct {
	RemoveVolumes bool
}

type LogsOptions struct {
	Follow bool
	Tail   int
}

type Container struct {
	Name   string
	Image  string
	State  string // running | exited | created | ...
	Status string // human-readable
}
