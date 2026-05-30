package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/config"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/paths"
	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/secrets"
	"github.com/spf13/cobra"
)

func newApplyCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Materialize .env, .secrets.env, and expand config/*.template files",
		RunE:  runApply,
	}
}

func runApply(cmd *cobra.Command, _ []string) error {
	l, err := resolveLayout(cmd)
	if err != nil {
		return err
	}
	c, err := loadValidConfig(l)
	if err != nil {
		return err
	}
	vault, err := secrets.Load(l.VaultFile())
	if err != nil {
		return fmt.Errorf("load secrets (run `iyzitrace init` first?): %w", err)
	}

	env, err := buildEnvMap(c, l)
	if err != nil {
		return err
	}
	if err := writeEnvFile(l.EnvFile(), env, 0o644); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	if err := os.WriteFile(l.SecretsEnvFile(), vault.EnvFile(), 0o600); err != nil {
		return fmt.Errorf("write .secrets.env: %w", err)
	}

	// Expand every *.template under <install>/config/ into its sibling using
	// vault values. Service tools (loki/tempo/etc.) can use their *-config.expand-env
	// flags if they prefer runtime expansion, but pre-expanding keeps a single
	// rendering model.
	if err := renderConfigTemplates(l.ConfigDir(), vault.AsMap()); err != nil {
		return fmt.Errorf("render config templates: %w", err)
	}

	for _, sub := range serviceDataDirs() {
		if err := os.MkdirAll(filepath.Join(c.Deployment.DataDir, sub), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}
	if err := os.MkdirAll(l.CertsDir(), 0o755); err != nil {
		return err
	}

	if err := stampApplyHash(l); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "applied -> %s\n", l.InstallDir)
	return nil
}

// buildEnvMap returns the variables that compose substitutes at `up` time.
func buildEnvMap(c *config.Config, l paths.Layout) (map[string]string, error) {
	out := map[string]string{
		"NETWORK":    c.Deployment.Network,
		"DATA_DIR":   c.Deployment.DataDir,
		"CERTS_DIR":  l.CertsDir(),
		"HTTP_PORT":  strconv.Itoa(c.Deployment.HTTPPort),
		"HTTPS_PORT": strconv.Itoa(c.Deployment.HTTPSPort),
		"DOMAIN":     c.Deployment.Domain,
	}
	for name, svc := range c.Services {
		out[strings.ToUpper(name)+"_IMAGE"] = svc.Image
		if svc.Build != "" {
			abs, err := filepath.Abs(svc.Build)
			if err != nil {
				return nil, fmt.Errorf("resolve build path for %s: %w", name, err)
			}
			out[strings.ToUpper(name)+"_BUILD"] = abs
		}
	}
	return out, nil
}

func writeEnvFile(path string, env map[string]string, mode os.FileMode) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&buf, "%s=%s\n", k, env[k])
	}
	return os.WriteFile(path, []byte(buf.String()), mode)
}

func renderConfigTemplates(root string, env map[string]string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".template") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		expanded := os.Expand(string(raw), func(k string) string { return env[k] })
		dst := strings.TrimSuffix(path, ".template")
		if err := os.WriteFile(dst, []byte(expanded), 0o600); err != nil {
			return err
		}
		// WriteFile keeps the existing mode on overwrite; force 0o600 since
		// these files contain secrets.
		return os.Chmod(dst, 0o600)
	})
}

func serviceDataDirs() []string {
	return []string{
		"tempo", "loki", "prometheus", "seaweedfs",
		"thanos-store", "thanos-compactor",
		"auth", "lawrence", "inventory",
		"alertmanager",
	}
}

// stampApplyHash records sha256(iyzitrace.yaml + BUNDLE_VERSION) so `up` can
// later detect "apply hasn't been run since the config changed".
func stampApplyHash(l paths.Layout) error {
	cfgBytes, err := os.ReadFile(l.ConfigFile())
	if err != nil {
		return err
	}
	verBytes, _ := os.ReadFile(l.BundleVersionFile())

	h := sha256.New()
	h.Write(cfgBytes)
	h.Write(verBytes)
	hash := hex.EncodeToString(h.Sum(nil))

	prev := map[string]any{}
	if b, err := os.ReadFile(l.StateFile()); err == nil {
		_ = json.Unmarshal(b, &prev)
	}
	prev["last_apply_hash"] = hash
	prev["bundle_version"] = strings.TrimSpace(string(verBytes))
	out, err := json.MarshalIndent(prev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.StateFile(), out, 0o644)
}
