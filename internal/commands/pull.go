package commands

import (
	"fmt"
	"os"
	"strings"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"

	"github.com/spf13/cobra"
)

// shellQuote single-quotes a value so it survives `source`/`eval` and direnv's
// dotenv parser intact, including spaces, $, and quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func newPullCmd() *cobra.Command {
	var (
		scope     scopeFlags
		shellMode bool
		outFile   string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull env vars & secrets into a file or shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, session, err := auth.LoadSessionConfig()
			if err != nil {
				return err
			}
			project, env, err := scope.resolve()
			if err != nil {
				return err
			}

			client := api.NewClient(cfg.APIURL).WithToken(session.Token).WithSessionKey(session.SessionKey)
			vars, err := client.Pull(cmd.Context(), project, env)
			if err != nil {
				return err
			}

			if shellMode {
				for k, v := range vars {
					fmt.Printf("export %s=%s\n", k, shellQuote(v))
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
				fmt.Fprintf(f, "export %s=%s\n", k, shellQuote(v))
			}
			fmt.Printf("Wrote %s\n", path)
			return nil
		},
	}
	addScopeFlags(cmd, &scope)
	cmd.Flags().BoolVar(&shellMode, "shell", false, "emit exports to stdout for direnv/.envrc")
	cmd.Flags().StringVar(&outFile, "out", "", "output file (default: .env)")
	return cmd
}
