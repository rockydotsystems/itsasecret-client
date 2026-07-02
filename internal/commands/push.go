package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var (
		project string
		env     string
		file    string
		secrets []string
	)
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push local .env to a remote environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if env == "" {
				env = "production"
			}
			if file == "" {
				file = ".env"
			}

			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("opening %s: %w", file, err)
			}
			defer f.Close()

			secretSet := make(map[string]bool, len(secrets))
			for _, s := range secrets {
				secretSet[s] = true
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			ctx := cmd.Context()

			scanner := bufio.NewScanner(f)
			var count int
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				line = strings.TrimPrefix(line, "export ")
				idx := strings.Index(line, "=")
				if idx < 0 {
					continue
				}
				key := strings.TrimSpace(line[:idx])
				value := unquote(strings.TrimSpace(line[idx+1:]))

				if secretSet[key] {
					if err := client.SetSecret(ctx, project, env, key, value); err != nil {
						return fmt.Errorf("set secret %s: %w", key, err)
					}
				} else {
					if err := client.SetVar(ctx, project, env, key, value); err != nil {
						return fmt.Errorf("set var %s: %w", key, err)
					}
				}
				count++
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("reading %s: %w", file, err)
			}
			fmt.Printf("Pushed %d entries to %s/%s\n", count, project, env)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().StringVar(&file, "file", "", "input file (default: .env)")
	cmd.Flags().StringSliceVar(&secrets, "secret", nil, "keys to treat as secrets (encrypted); may be repeated or comma-separated")
	return cmd
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
