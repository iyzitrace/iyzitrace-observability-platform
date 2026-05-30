// Package paths resolves where iyzitrace stores its install dir, data, and secrets.
//
// Resolution rules:
//   - explicit --install-dir / IYZITRACE_INSTALL_DIR wins
//   - else if running as root (uid 0): system FHS layout
//   - else: per-user under $HOME/.iyzitrace
//
// The install dir is the runtime root. The extracted bundle lives at the same
// level (docker-compose.yml + config/ at the root), so docker compose can be
// run with `--env-file <install>/.env -f <install>/docker-compose.yml` and
// every relative path in compose resolves against <install>.
package paths

import (
	"errors"
	"os"
	"path/filepath"
)

type Layout struct {
	InstallDir string // compose, config/, .env, .secrets.env, state.json
	DataDir    string // persistent volumes root (overridden by Deployment.DataDir at apply time)
	SecretsDir string // vault.json
	System     bool   // true when running under FHS layout
}

func Resolve(installDirOverride string) (Layout, error) {
	if v := os.Getenv("IYZITRACE_INSTALL_DIR"); installDirOverride == "" && v != "" {
		installDirOverride = v
	}
	if installDirOverride != "" {
		abs, err := filepath.Abs(installDirOverride)
		if err != nil {
			return Layout{}, err
		}
		return Layout{
			InstallDir: abs,
			DataDir:    filepath.Join(abs, "data"),
			SecretsDir: filepath.Join(abs, "secrets"),
		}, nil
	}

	if os.Geteuid() == 0 {
		return Layout{
			InstallDir: "/etc/iyzitrace",
			DataDir:    "/var/lib/iyzitrace/data",
			SecretsDir: "/var/lib/iyzitrace/secrets",
			System:     true,
		}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, errors.Join(errors.New("cannot resolve user home dir"), err)
	}
	root := filepath.Join(home, ".iyzitrace")
	return Layout{
		InstallDir: root,
		DataDir:    filepath.Join(root, "data"),
		SecretsDir: filepath.Join(root, "secrets"),
	}, nil
}

func (l Layout) ConfigFile() string     { return filepath.Join(l.InstallDir, "iyzitrace.yaml") }
func (l Layout) ConfigDir() string      { return filepath.Join(l.InstallDir, "config") }
func (l Layout) ComposeFile() string    { return filepath.Join(l.InstallDir, "docker-compose.yml") }
func (l Layout) EnvFile() string        { return filepath.Join(l.InstallDir, ".env") }
func (l Layout) SecretsEnvFile() string { return filepath.Join(l.InstallDir, ".secrets.env") }
func (l Layout) StateFile() string      { return filepath.Join(l.InstallDir, "state.json") }
func (l Layout) CertsDir() string       { return filepath.Join(l.InstallDir, "certs") }
func (l Layout) VaultFile() string      { return filepath.Join(l.SecretsDir, "vault.json") }
func (l Layout) BundleVersionFile() string {
	return filepath.Join(l.InstallDir, "BUNDLE_VERSION")
}
