package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "iyzitrace",
		Short:         "Install, configure, and operate the iyzitrace observability platform",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().String("install-dir", "", "override install dir (default: auto by uid)")

	cmd.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newConfigCmd(),
		newApplyCmd(),
		newUpCmd(),
		newDownCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newSecretsCmd(),
	)
	return cmd
}
