package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newStatusCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the state of every component (from `docker compose ps`)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := backendFor(l)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			containers, err := b.PS(ctx)
			if err != nil {
				return err
			}
			if len(containers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no containers found (run `iyzitrace up`)")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATE\tSTATUS\tIMAGE")
			for _, c := range containers {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Name, c.State, c.Status, c.Image)
			}
			return tw.Flush()
		},
	}
}
