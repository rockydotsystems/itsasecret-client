package commands

import (
	"fmt"
	"os"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/config"

	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var (
		project   string
		env       string
		shellMode bool
		outFile   string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull env vars & secrets into a file or shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.SessionToken == "" {
				return fmt.Errorf("not logged in — run `itsasecret login` first")
			}
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if env == "" {
				env = "production"
			}

			client := api.NewClient(cfg.APIURL).WithToken(cfg.SessionToken)
			vars, err := client.Pull(cmd.Context(), project, env)
			if err != nil {
				return err
			}

			if shellMode {
				for k, v := range vars {
					fmt.Printf("export %s=%s\n", k, v)
				}
				return nil
			}

			path := outFile
			if path == "" {
				path = ".env"
			}
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			for k, v := range vars {
				fmt.Fprintf(f, "export %s=%s\n", k, v)
			}
			fmt.Printf("Wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID (required)")
	cmd.Flags().StringVar(&env, "env", "", "environment name (default: production)")
	cmd.Flags().BoolVar(&shellMode, "shell", false, "emit exports to stdout for direnv/.envrc")
	cmd.Flags().StringVar(&outFile, "out", "", "output file (default: .env)")
	return cmd
}
