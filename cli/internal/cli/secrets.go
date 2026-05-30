package cli

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/iyzitrace/iyzitrace-observability-platform/cli/internal/secrets"
	"github.com/spf13/cobra"
)

func newSecretsCmdReal() *cobra.Command {
	c := &cobra.Command{Use: "secrets", Short: "Manage platform secrets vault"}
	c.AddCommand(
		newSecretsList(),
		newSecretsShow(),
		newSecretsRotate(),
		newSecretsSet(),
	)
	return c
}

// reflective accessor for vault fields by user-facing name (snake_case json tag).
func vaultMap(v *secrets.Vault) map[string]string {
	out := map[string]string{}
	t := reflect.TypeOf(*v)
	rv := reflect.ValueOf(*v)
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		out[tag] = rv.Field(i).String()
	}
	return out
}

func vaultSet(v *secrets.Vault, name, value string) error {
	t := reflect.TypeOf(v).Elem()
	rv := reflect.ValueOf(v).Elem()
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag == name {
			rv.Field(i).SetString(value)
			return nil
		}
	}
	return fmt.Errorf("unknown secret name: %q", name)
}

func newSecretsList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List secret names (never values)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			v, err := secrets.Load(l.VaultFile())
			if err != nil {
				return err
			}
			m := vaultMap(v)
			names := make([]string, 0, len(m))
			for n := range m {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s %d bytes\n", n, len(m[n]))
			}
			return nil
		},
	}
}

func newSecretsShow() *cobra.Command {
	c := &cobra.Command{
		Use:   "show <name>",
		Short: "Print one secret value (requires --yes-i-really-want-this)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			confirm, _ := cmd.Flags().GetBool("yes-i-really-want-this")
			if !confirm {
				return fmt.Errorf("refusing to print secret without --yes-i-really-want-this")
			}
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			v, err := secrets.Load(l.VaultFile())
			if err != nil {
				return err
			}
			m := vaultMap(v)
			val, ok := m[args[0]]
			if !ok {
				return fmt.Errorf("unknown secret %q", args[0])
			}
			fmt.Fprintln(cmd.OutOrStdout(), val)
			return nil
		},
	}
	c.Flags().Bool("yes-i-really-want-this", false, "required to actually print the value")
	return c
}

func newSecretsRotate() *cobra.Command {
	return &cobra.Command{
		Use:   "rotate [name...]",
		Short: "Generate a fresh value for one, many, or all secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			v, err := secrets.Load(l.VaultFile())
			if err != nil {
				return err
			}
			fresh := secrets.GenerateAll()
			freshMap := vaultMap(fresh)

			var rotated []string
			if len(args) == 0 {
				v = fresh
				for k := range freshMap {
					rotated = append(rotated, k)
				}
			} else {
				for _, name := range args {
					nv, ok := freshMap[name]
					if !ok {
						return fmt.Errorf("unknown secret %q", name)
					}
					if err := vaultSet(v, name, nv); err != nil {
						return err
					}
					rotated = append(rotated, name)
				}
			}
			if err := secrets.Save(l.VaultFile(), v); err != nil {
				return err
			}
			sort.Strings(rotated)
			for _, n := range rotated {
				fmt.Fprintf(cmd.OutOrStdout(), "rotated %s\n", n)
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "run `iyzitrace apply && iyzitrace up` to roll the new values onto running services")
			return nil
		},
	}
}

func newSecretsSet() *cobra.Command {
	c := &cobra.Command{
		Use:   "set <name>=<value>",
		Short: "Override a secret with a known value (e.g. preserve a JWT_SECRET on upgrade)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, val, ok := strings.Cut(args[0], "=")
			if !ok {
				return fmt.Errorf("expected name=value, got %q", args[0])
			}
			fromStdin, _ := cmd.Flags().GetBool("from-stdin")
			if fromStdin {
				b, err := os.ReadFile("/dev/stdin")
				if err != nil {
					return err
				}
				val = strings.TrimRight(string(b), "\n")
			}
			l, err := resolveLayout(cmd)
			if err != nil {
				return err
			}
			v, err := secrets.Load(l.VaultFile())
			if err != nil {
				return err
			}
			if err := vaultSet(v, name, val); err != nil {
				return err
			}
			if err := secrets.Save(l.VaultFile(), v); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s\n", name)
			return nil
		},
	}
	c.Flags().Bool("from-stdin", false, "read the value from stdin instead of the argument (avoids shell history)")
	return c
}
