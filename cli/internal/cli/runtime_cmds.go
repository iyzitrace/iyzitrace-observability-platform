package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/paths"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/runtime"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/runtime/compose"
	"github.com/spf13/cobra"
)

func backendFor(l paths.Layout) (*compose.Backend, error) {
	if _, err := os.Stat(l.ComposeFile()); err != nil {
		return nil, fmt.Errorf("compose file missing at %s — run `iyzitrace init` first", l.ComposeFile())
	}
	b := compose.New(l.ComposeFile())
	b.WorkingDir = l.InstallDir
	if _, err := os.Stat(l.EnvFile()); err == nil {
		b.EnvFile = l.EnvFile()
	}
	return b, nil
}

func newUpCmdReal() *cobra.Command {
	c := &cobra.Command{
		Use:   "up [service...]",
		Short: "Start the platform (or specific services)",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := backendFor(l)
			if err != nil {
				return err
			}
			detach, _ := cmd.Flags().GetBool("detach")
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if err := compose.EnsureAvailable(ctx); err != nil {
				return err
			}
			return b.Up(ctx, args, runtime.UpOptions{Detach: detach})
		},
	}
	c.Flags().BoolP("detach", "d", true, "run containers in the background")
	return c
}

func newDownCmdReal() *cobra.Command {
	c := &cobra.Command{
		Use:   "down",
		Short: "Stop the platform",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := backendFor(l)
			if err != nil {
				return err
			}
			purge, _ := cmd.Flags().GetBool("purge")
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if err := b.Down(ctx, runtime.DownOptions{RemoveVolumes: purge}); err != nil {
				return err
			}
			if purge {
				if err := os.RemoveAll(l.DataDir); err != nil {
					return fmt.Errorf("remove data dir: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "purged %s\n", l.DataDir)
			}
			return nil
		},
	}
	c.Flags().Bool("purge", false, "also delete data and secrets (irreversible)")
	return c
}

func newRestartCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [service...]",
		Short: "Restart services",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := backendFor(l)
			if err != nil {
				return err
			}
			return b.Restart(cmd.Context(), args)
		},
	}
}

func newLogsCmdReal() *cobra.Command {
	c := &cobra.Command{
		Use:   "logs [service]",
		Short: "Stream service logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := backendFor(l)
			if err != nil {
				return err
			}
			follow, _ := cmd.Flags().GetBool("follow")
			tail, _ := cmd.Flags().GetInt("tail")
			svc := ""
			if len(args) == 1 {
				svc = args[0]
			}
			return b.Logs(cmd.Context(), svc, runtime.LogsOptions{Follow: follow, Tail: tail})
		},
	}
	c.Flags().BoolP("follow", "f", false, "follow log output")
	c.Flags().Int("tail", 100, "number of lines to show from the end of the logs")
	return c
}
