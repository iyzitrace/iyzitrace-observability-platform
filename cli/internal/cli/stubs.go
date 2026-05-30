package cli

import (
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/paths"
	"github.com/spf13/cobra"
)

func resolveLayout(cmd *cobra.Command) (paths.Layout, error) {
	override, _ := cmd.Flags().GetString("install-dir")
	return paths.Resolve(override)
}

func newInitCmd() *cobra.Command    { return newInitCmdReal() }
func newConfigCmd() *cobra.Command  { return newConfigCmdReal() }
func newApplyCmd() *cobra.Command   { return newApplyCmdReal() }
func newUpCmd() *cobra.Command      { return newUpCmdReal() }
func newDownCmd() *cobra.Command    { return newDownCmdReal() }
func newRestartCmd() *cobra.Command { return newRestartCmdReal() }
func newStatusCmd() *cobra.Command  { return newStatusCmdReal() }
func newLogsCmd() *cobra.Command    { return newLogsCmdReal() }
func newSecretsCmd() *cobra.Command { return newSecretsCmdReal() }
