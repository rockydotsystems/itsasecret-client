package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"itsasecret.dev/cli/internal/api"
	"itsasecret.dev/cli/internal/auth"
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

The only setting today is the server URL. It is set once per machine
(stored in the global config file); a repo can override it by committing a
` + "`url =`" + ` line in ` + localcfg.ProjectFile + ` - useful for self-hosted servers.
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
		Use:       "get url",
		Short:     "Print the server URL the CLI would use here",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"url"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, files, err := loadConfigAndFiles()
			if err != nil {
				return err
			}
			if files.URL != "" {
				say(out, "%s (from %s)\n", files.URL, files.ProjectPath)
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
		Use:   "set url <url>",
		Short: "Set the server URL once - globally, or in " + localcfg.ProjectFile,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "url" {
				return fmt.Errorf("unknown setting %q (only \"url\" exists)", args[0])
			}
			serverURL, err := normalizeServerURL(args[1])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			cfg, files, err := loadConfigAndFiles()
			if err != nil {
				return err
			}
			if inProject {
				if err := setProjectURL(out, files, serverURL); err != nil {
					return err
				}
			} else if err := setGlobalURL(out, cfg, serverURL); err != nil {
				return err
			}
			reportLoginStatus(cmd.Context(), out, cfg, serverURL)
			return nil
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

func setGlobalURL(out io.Writer, cfg *config.Config, serverURL string) error {
	cfg.APIURL = serverURL
	if err := cfg.Save(); err != nil {
		return err
	}
	say(out, "Server URL set to %s for this machine.\n", serverURL)
	return nil
}

// sessionStatus verifies a stored session for serverURL against the server
// (via /api/auth/me) and describes the result, instead of guessing at a
// login hint.
func sessionStatus(ctx context.Context, cfg *config.Config, serverURL string) string {
	stored, ok := cfg.Session(serverURL)
	if !ok || stored.Token == "" {
		return "not logged in - run `shh login`"
	}
	if stored.Expired() {
		return "session idle-expired - the next command asks for your master password"
	}
	if err := requireSecureURL(serverURL); err != nil {
		return fmt.Sprintf("insecure server URL - session not verified (%v)", err)
	}
	session, err := auth.SessionFor(cfg, serverURL)
	if err != nil {
		return fmt.Sprintf("stored session unreadable (%v)", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	email, err := clientFor(cfg, serverURL, session).Me(ctx)
	switch {
	case err == nil:
		return "logged in as " + email + " (session verified)"
	case errors.Is(err, api.ErrUnauthorized):
		return "session expired - the next command asks for your master password"
	default:
		return fmt.Sprintf("couldn't verify session (%v)", err)
	}
}

func reportLoginStatus(ctx context.Context, out io.Writer, cfg *config.Config, serverURL string) {
	say(out, "Session: %s.\n", sessionStatus(ctx, cfg, serverURL))
}

func setProjectURL(out io.Writer, files *localcfg.Scope, serverURL string) error {
	if files.ProjectPath == "" {
		return fmt.Errorf("no %s found here - run `shh link` first, or set it globally without --project", localcfg.ProjectFile)
	}
	if err := localcfg.SaveURL(files.ProjectPath, serverURL); err != nil {
		return err
	}
	say(out, "Server URL set to %s in %s (commit this file).\n", serverURL, files.ProjectPath)
	return nil
}

// runConfigMenu is the interactive entry point: pick an action, then run its
// flow. New settings and actions slot in as menu options.
func runConfigMenu(cmd *cobra.Command) error {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()
	cfg, files, err := loadConfigAndFiles()
	if err != nil {
		return err
	}

	const (
		actionSetURL = iota
		actionShow
	)
	actions := []huh.Option[int]{
		huh.NewOption("set the server URL", actionSetURL),
		huh.NewOption("show the current configuration", actionShow),
	}
	action, err := selectIndex(cmd.Context(), in, out, "itsasecret CLI configuration - what do you want to do?", actions)
	if err != nil {
		return err
	}

	switch action {
	case actionShow:
		printConfigStatus(cmd.Context(), out, cfg, files)
		return nil
	case actionSetURL:
		return runSetURLMenu(cmd, cfg, files)
	}
	return nil
}

func printConfigStatus(ctx context.Context, out io.Writer, cfg *config.Config, files *localcfg.Scope) {
	say(out, "server url: %s (global)\n", cfg.APIURL)
	if files.URL != "" {
		say(out, "            %s (override from %s - commands here use this)\n", files.URL, files.ProjectPath)
	}
	if len(cfg.Sessions) == 0 {
		sayln(out, "sessions:   none - run `shh login`")
		return
	}
	urls := make([]string, 0, len(cfg.Sessions))
	for u := range cfg.Sessions {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	label := "sessions:  "
	for _, u := range urls {
		say(out, "%s %s | %s\n", label, u, sessionStatus(ctx, cfg, u))
		label = "           "
	}
}

// runSetURLMenu interactively sets the server URL: pick the scope (machine
// vs project), then enter the URL.
func runSetURLMenu(cmd *cobra.Command, cfg *config.Config, files *localcfg.Scope) error {
	in, out := cmd.InOrStdin(), cmd.OutOrStdout()

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
		var err error
		scope, err = selectIndex(cmd.Context(), in, out, "Where should the server URL be set?", opts)
		if err != nil {
			return err
		}
	}

	current := cfg.APIURL
	if scope == scopeProject && files.URL != "" {
		current = files.URL
	}
	serverURL := current
	field := huh.NewInput().
		Title("Server URL").
		Description("Set once - every command uses it from now on.").
		Value(&serverURL).
		Validate(validateServerURL)
	if err := runField(cmd.Context(), in, out, field); err != nil {
		return err
	}
	serverURL, err := normalizeServerURL(serverURL)
	if err != nil {
		return err
	}

	if scope == scopeProject {
		if err := setProjectURL(out, files, serverURL); err != nil {
			return err
		}
	} else if err := setGlobalURL(out, cfg, serverURL); err != nil {
		return err
	}
	reportLoginStatus(cmd.Context(), out, cfg, serverURL)
	return nil
}

func validateServerURL(s string) error {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("enter a full URL like https://itsasecret.dev")
	}
	if u.User != nil {
		return fmt.Errorf("server URL must not contain userinfo (user:pass@)")
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
			return fmt.Errorf("insecure http:// URL for non-loopback host %q - use https", host)
		}
	}
	return nil
}

// normalizeServerURL validates and canonicalizes the URL (no trailing slash -
// the API client appends absolute paths).
func normalizeServerURL(s string) (string, error) {
	if err := validateServerURL(s); err != nil {
		return "", err
	}
	return strings.TrimRight(strings.TrimSpace(s), "/"), nil
}
