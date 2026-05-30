package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	BinaryVersion = "dev"
	BinaryCommit  = ""
	BinaryDate    = ""
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, commit, date := BinaryVersion, BinaryCommit, BinaryDate
			if commit == "" {
				if info, ok := debug.ReadBuildInfo(); ok {
					for _, s := range info.Settings {
						if s.Key == "vcs.revision" {
							commit = s.Value
						}
						if s.Key == "vcs.time" {
							date = s.Value
						}
					}
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "iyzitrace %s\n", v)
			fmt.Fprintf(cmd.OutOrStdout(), "  bundle: %s\n", DefaultBundleVersion)
			if commit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  commit: %s\n", commit)
			}
			if date != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  built:  %s\n", date)
			}
			return nil
		},
	}
}
