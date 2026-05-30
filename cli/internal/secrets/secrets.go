// Package secrets generates and stores platform secrets.
//
// v1 stores them as a JSON file with mode 0600. Phase 6 wraps this in age
// encryption with the same on-disk layout (vault.enc + key.txt).
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Vault struct {
	LokiAccessKey   string `json:"loki_access_key"`
	LokiSecretKey   string `json:"loki_secret_key"`
	TempoAccessKey  string `json:"tempo_access_key"`
	TempoSecretKey  string `json:"tempo_secret_key"`
	ThanosAccessKey string `json:"thanos_access_key"`
	ThanosSecretKey string `json:"thanos_secret_key"`
	JWTSecret       string `json:"jwt_secret"`
}

func GenerateAll() *Vault {
	return &Vault{
		LokiAccessKey:   randHex(16),
		LokiSecretKey:   randHex(32),
		TempoAccessKey:  randHex(16),
		TempoSecretKey:  randHex(32),
		ThanosAccessKey: randHex(16),
		ThanosSecretKey: randHex(32),
		JWTSecret:       randHex(32),
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failures are catastrophic, not recoverable
	}
	return hex.EncodeToString(b)
}

// Save writes the vault to path with mode 0600. Parent dir is created.
func Save(path string, v *Vault) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Load(path string) (*Vault, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v Vault
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("parse vault: %w", err)
	}
	return &v, nil
}

// LoadOrInit returns an existing vault or generates and saves a new one.
func LoadOrInit(path string) (*Vault, bool, error) {
	if v, err := Load(path); err == nil {
		return v, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	v := GenerateAll()
	if err := Save(path, v); err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// AsMap returns the vault as a flat string map, keyed by the env-var names
// used in templates and the .secrets.env file.
func (v *Vault) AsMap() map[string]string {
	return map[string]string{
		"LOKI_ACCESS_KEY":   v.LokiAccessKey,
		"LOKI_SECRET_KEY":   v.LokiSecretKey,
		"TEMPO_ACCESS_KEY":  v.TempoAccessKey,
		"TEMPO_SECRET_KEY":  v.TempoSecretKey,
		"THANOS_ACCESS_KEY": v.ThanosAccessKey,
		"THANOS_SECRET_KEY": v.ThanosSecretKey,
		"JWT_SECRET":        v.JWTSecret,
	}
}

// EnvFile renders the vault as a `.secrets.env` for compose's env_file:
// directive. Keys are sorted for deterministic output.
func (v *Vault) EnvFile() []byte {
	m := v.AsMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	return []byte(b.String())
}
