package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/config"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/paths"
	"github.com/spf13/cobra"
)

func newConfigCmdReal() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "View and edit iyzitrace.yaml"}
	c.AddCommand(newConfigShow(), newConfigEdit(), newConfigValidate())
	return c
}

func newConfigShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print iyzitrace.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(l.ConfigFile())
			if err != nil {
				return err
			}
			_, err = io.Copy(cmd.OutOrStdout(), strings.NewReader(string(b)))
			return err
		},
	}
}

func newConfigEdit() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open iyzitrace.yaml in $EDITOR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			c := exec.Command(editor, l.ConfigFile())
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			return c.Run()
		},
	}
}

func newConfigValidate() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate iyzitrace.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			c, err := config.Load(l.ConfigFile())
			if err != nil {
				return err
			}
			if err := c.Validate(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}

func loadValidConfig(l paths.Layout) (*config.Config, error) {
	c, err := config.Load(l.ConfigFile())
	if err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
