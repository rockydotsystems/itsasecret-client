package commands

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"itsasecret.dev/cli/internal/config"
	"itsasecret.dev/cli/internal/localcfg"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View or change CLI configuration",
		Long: `View or change CLI configuration. Run bare for an interactive menu.

The only setting today is the API server. It is set once per machine
(stored in the global config file); a repo can override it by committing an
` + "`api =`" + ` line in ` + localcfg.ProjectFile + ` — useful for self-hosted servers.
Every command resolves it as: ` + localcfg.ProjectFile + ` > global > default.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigMenu(cmd)
		},
	}
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get api",
		Short: "Print the API server the CLI would use here",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"api"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, files, err := loadConfigAndFiles()
			if err != nil {
				return err
			}
			if files.API != "" {
				say(out, "%s (from %s)\n", files.API, files.ProjectPath)
			} else {
				say(out, "%s (this machine's global config)\n", cfg.APIURL)
			}
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	var inProject bool
	cmd := &cobra.Command{
		Use:   "set api <url>",
		Short: "Set the API server once — globally, or in " + localcfg.ProjectFile,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "api" {
				return fmt.Errorf("unknown setting %q (only \"api\" exists)", args[0])
			}
			apiURL, err := normalizeAPIURL(args[1])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			cfg, files, err := loadConfigAndFiles()
			if err != nil {
				return err
			}
			if inProject {
				return setProjectAPI(out, files, apiURL)
			}
			return setGlobalAPI(out, cfg, apiURL)
		},
	}
	cmd.Flags().BoolVar(&inProject, "project", false, "save in the resolved "+localcfg.ProjectFile+" instead of the global config")
	return cmd
}

func loadConfigAndFiles() (*config.Config, *localcfg.Scope, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	files, err := localcfg.Find(cwd)
	if err != nil {
		return nil, nil, err
	}
	return cfg, files, nil
}

func setGlobalAPI(out io.Writer, cfg *config.Config, apiURL string) error {
	cfg.APIURL = apiURL
	if err := cfg.Save(); err != nil {
		return err
	}
	say(out, "API server set to %s for this machine.\n", apiURL)
	sayln(out, "Run `shh login` to authenticate against it.")
	return nil
}

func setProjectAPI(out io.Writer, files *localcfg.Scope, apiURL string) error {
	if files.ProjectPath == "" {
		return fmt.Errorf("no %s found here — run `shh link` first, or set it globally without --project", localcfg.ProjectFile)
	}
	if err := localcfg.SaveAPI(files.ProjectPath, apiURL); err != nil {
		return err
	}
	say(out, "API server set to %s in %s (commit this file).\n", apiURL, files.ProjectPath)
	return nil
}

// runConfigMenu interactively sets the API server: pick the scope (machine
// vs project), then enter the URL.
func runConfigMenu(cmd *cobra.Command) error {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	cfg, files, err := loadConfigAndFiles()
	if err != nil {
		return err
	}

	sayln(out, "itsasecret CLI configuration")
	say(out, "api server: %s (global)\n", cfg.APIURL)
	if files.API != "" {
		say(out, "            %s (override from %s)\n", files.API, files.ProjectPath)
	}
	sayln(out)

	const (
		scopeGlobal = iota
		scopeProject
	)
	scope := scopeGlobal
	if files.ProjectPath != "" {
		opts := []huh.Option[int]{
			huh.NewOption("this machine (global config)", scopeGlobal),
			huh.NewOption(fmt.Sprintf("this project (%s, committed)", files.ProjectPath), scopeProject),
		}
		scope, err = selectIndex(cmd.Context(), in, out, "Where should the API server be set?", opts)
		if err != nil {
			return err
		}
	}

	current := cfg.APIURL
	if scope == scopeProject && files.API != "" {
		current = files.API
	}
	apiURL := current
	field := huh.NewInput().
		Title("API server URL").
		Description("Set once — every command uses it from now on.").
		Value(&apiURL).
		Validate(validateAPIURL)
	if err := runField(cmd.Context(), in, out, field); err != nil {
		return err
	}
	apiURL, err = normalizeAPIURL(apiURL)
	if err != nil {
		return err
	}

	if scope == scopeProject {
		return setProjectAPI(out, files, apiURL)
	}
	return setGlobalAPI(out, cfg, apiURL)
}

func validateAPIURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("enter a full URL like https://itsasecret.dev")
	}
	return nil
}

// normalizeAPIURL validates and canonicalizes the URL (no trailing slash —
// the API client appends absolute paths).
func normalizeAPIURL(s string) (string, error) {
	if err := validateAPIURL(s); err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(s), "/"), nil
}
