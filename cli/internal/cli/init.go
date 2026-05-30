package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/bundle"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/secrets"
	"github.com/spf13/cobra"
)

// DefaultBundleVersion is the bundle version this CLI was built against.
// Set via -ldflags at release time so it tracks bundle/BUNDLE_VERSION.
// Defaults to "dev" for local builds that haven't pinned a release.
var DefaultBundleVersion = "dev"

func newInitCmdReal() *cobra.Command {
	c := &cobra.Command{
		Use:   "init",
		Short: "Fetch the bundle, generate secrets, and prepare the install dir",
		RunE:  runInit,
	}
	c.Flags().String("bundle-url", "", "override bundle tarball URL (default: GitHub release)")
	c.Flags().String("bundle-version", "", "bundle version to fetch (default: pinned in CLI)")
	c.Flags().Bool("require-checksum", false, "fail if no .sha256 sibling is found")
	c.Flags().Bool("force", false, "overwrite an existing iyzitrace.yaml")
	return c
}

func runInit(cmd *cobra.Command, _ []string) error {
	l, err := resolveLayout(cmd)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if err := os.MkdirAll(l.InstallDir, 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	bundleURL, _ := cmd.Flags().GetString("bundle-url")
	bundleVersion, _ := cmd.Flags().GetString("bundle-version")
	requireSum, _ := cmd.Flags().GetBool("require-checksum")
	if bundleVersion == "" {
		bundleVersion = DefaultBundleVersion
	}
	if bundleURL == "" {
		bundleURL = fmt.Sprintf(bundle.DefaultGitHubReleaseURLTemplate, bundleVersion, bundleVersion)
	}

	fmt.Fprintf(out, "fetching bundle from %s\n", bundleURL)
	v, err := bundle.FetchAndExtract(bundleURL, l.InstallDir, requireSum)
	if err != nil {
		return fmt.Errorf("bundle: %w", err)
	}
	fmt.Fprintf(out, "extracted bundle %s -> %s\n", v, l.InstallDir)

	// Promote iyzitrace.yaml.default → iyzitrace.yaml on first run (or --force).
	cfgPath := l.ConfigFile()
	force, _ := cmd.Flags().GetBool("force")
	defaultPath := filepath.Join(l.InstallDir, "iyzitrace.yaml.default")
	if _, err := os.Stat(cfgPath); err == nil && !force {
		fmt.Fprintf(out, "iyzitrace.yaml already exists, keeping it (use --force to overwrite)\n")
	} else {
		b, err := os.ReadFile(defaultPath)
		if err != nil {
			return fmt.Errorf("read default config: %w", err)
		}
		if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", cfgPath)
	}

	if _, created, err := secrets.LoadOrInit(l.VaultFile()); err != nil {
		return fmt.Errorf("secrets: %w", err)
	} else if created {
		fmt.Fprintf(out, "generated secrets at %s\n", l.VaultFile())
	} else {
		fmt.Fprintf(out, "kept existing secrets at %s\n", l.VaultFile())
	}

	fmt.Fprintf(out, "\nready. next:\n")
	fmt.Fprintf(out, "  iyzitrace config show\n")
	fmt.Fprintf(out, "  iyzitrace apply\n")
	fmt.Fprintf(out, "  iyzitrace up\n")
	return nil
}
