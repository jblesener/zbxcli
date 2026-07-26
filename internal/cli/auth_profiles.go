package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jblesener/zbxcli/internal/profile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newProfileCmd(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Inspect saved connection profiles"}
	cmd.AddCommand(newProfileListCmd(deps))
	return cmd
}

type profileListOptions struct{ output string }

func newProfileListCmd(deps Dependencies) *cobra.Command {
	var opts profileListOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List saved connection profiles",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runProfileList(cmd, deps, opts) },
	}
	cmd.Flags().StringVar(&opts.output, "output", "table", "Output format: table or json")
	return cmd
}

type profileSummary struct {
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Username      string     `json:"username,omitempty"`
	APIVersion    string     `json:"api_version"`
	TokenID       string     `json:"token_id"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	ExpiryStatus  string     `json:"expiry_status"`
	TLSThumbprint string     `json:"tls_thumbprint,omitempty"`
}

func runProfileList(cmd *cobra.Command, deps Dependencies, opts profileListOptions) error {
	if deps.Store == nil {
		return errors.New("zbxcli profile dependencies are not configured")
	}
	output := strings.ToLower(strings.TrimSpace(opts.output))
	if output != "table" && output != "json" {
		return fmt.Errorf("unsupported --output %q; use table or json", opts.output)
	}
	cfg, err := deps.Store.Load()
	if err != nil {
		return err
	}
	summaries := profileSummaries(cfg, time.Now())
	if output == "json" {
		raw, err := json.Marshal(summaries)
		if err != nil {
			return fmt.Errorf("encode profiles: %w", err)
		}
		return writeJSON(cmd, raw)
	}
	return writeProfileTable(cmd, summaries)
}

func profileSummaries(cfg profile.Config, now time.Time) []profileSummary {
	names := profile.Names(cfg)
	summaries := make([]profileSummary, 0, len(names))
	for _, name := range names {
		connection := cfg.Profiles[name]
		summaries = append(summaries, profileSummary{
			Name: name, URL: connection.URL, Username: connection.Username, APIVersion: connection.APIVersion,
			TokenID: connection.TokenID, ExpiresAt: connection.ExpiresAt, ExpiryStatus: profileExpiryStatus(connection.ExpiresAt, now), TLSThumbprint: connection.TLSThumbprint,
		})
	}
	return summaries
}

func profileExpiryStatus(expiresAt *time.Time, now time.Time) string {
	if expiresAt == nil {
		return "never"
	}
	if !expiresAt.After(now) {
		return "expired"
	}
	return "active"
}

func writeProfileTable(cmd *cobra.Command, summaries []profileSummary) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "PROFILE\tURL\tUSERNAME\tAPI VERSION\tEXPIRES\tSTATUS\tTLS THUMBPRINT"); err != nil {
		return err
	}
	for _, summary := range summaries {
		expires := "never"
		if summary.ExpiresAt != nil {
			expires = summary.ExpiresAt.Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", summary.Name, summary.URL, summary.Username, summary.APIVersion, expires, summary.ExpiryStatus, summary.TLSThumbprint); err != nil {
			return err
		}
	}
	return w.Flush()
}

type rotateOptions struct {
	username, profile, expiresIn, tokenName string
	showToken                               bool
}

func newRotateCmd(deps Dependencies) *cobra.Command {
	var opts rotateOptions
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate a saved profile's API token",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runRotate(cmd, deps, opts) },
	}
	cmd.Flags().StringVar(&opts.profile, "profile", "default", "Profile name")
	cmd.Flags().StringVar(&opts.username, "username", "", "Zabbix username (defaults to the saved username)")
	cmd.Flags().StringVar(&opts.expiresIn, "expires-in", "90d", "Token lifetime (for example 90d, 24h, or never)")
	cmd.Flags().StringVar(&opts.tokenName, "token-name", "", "Name for the Zabbix API token")
	cmd.Flags().BoolVar(&opts.showToken, "show-token", false, "Print the generated token to stdout")
	return cmd
}

func runRotate(cmd *cobra.Command, deps Dependencies, opts rotateOptions) error {
	if deps.Store == nil || deps.Store.Secrets == nil {
		return errors.New("zbxcli rotation dependencies are not configured")
	}
	if err := profile.ValidateName(opts.profile); err != nil {
		return err
	}
	cfg, err := deps.Store.Load()
	if err != nil {
		return err
	}
	connection, ok := cfg.Profiles[opts.profile]
	if !ok {
		return fmt.Errorf("profile %q is not configured; run zbxcli auth login --profile %s", opts.profile, opts.profile)
	}
	username := strings.TrimSpace(opts.username)
	if username == "" {
		username = connection.Username
	}
	return runLogin(cmd, deps, loginOptions{
		endpoint: connection.URL, username: username, profile: opts.profile, expiresIn: opts.expiresIn, tokenName: opts.tokenName,
		showToken: opts.showToken, rotate: true,
	})
}

type removeOptions struct {
	yes bool
}

func newRemoveCmd(deps Dependencies) *cobra.Command {
	var opts removeOptions
	cmd := &cobra.Command{
		Use:   "remove <profile>",
		Short: "Revoke and remove a saved profile",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runRemove(cmd, deps, opts, args[0]) },
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Skip the confirmation prompt")
	return cmd
}

func runRemove(cmd *cobra.Command, deps Dependencies, opts removeOptions, name string) error {
	if deps.Store == nil || deps.Store.Secrets == nil {
		return errors.New("zbxcli removal dependencies are not configured")
	}
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	cfg, err := deps.Store.Load()
	if err != nil {
		return err
	}
	connection, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q is not configured", name)
	}
	if strings.TrimSpace(connection.TokenID) == "" {
		return fmt.Errorf("profile %q has no saved API token ID and cannot be revoked", name)
	}
	writeProfileRemovalPreview(cmd.ErrOrStderr(), name, connection)
	if err := confirmProfileRemoval(cmd, opts.yes); err != nil {
		return err
	}
	token, err := deps.Store.Secrets.Get(name)
	if err != nil {
		return fmt.Errorf("load API token for profile %q: %w", name, err)
	}
	api, _, err := profileAPI(cmd, deps, name, "remove profile")
	if err != nil {
		return err
	}
	if err := api.DeleteToken(cmd.Context(), token, connection.TokenID); err != nil {
		return fmt.Errorf("revoke API token for profile %q: %w", name, err)
	}
	if err := removeStoredProfile(deps.Store, cfg, name, token); err != nil {
		return fmt.Errorf("API token was revoked, but local profile cleanup failed: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed profile %q.\n", name)
	return err
}

func writeProfileRemovalPreview(out io.Writer, name string, connection profile.Profile) {
	fmt.Fprintln(out, "Profile removal preview:")
	fmt.Fprintf(out, "  Profile: %s\n", name)
	fmt.Fprintf(out, "  URL: %s\n", connection.URL)
	if connection.Username != "" {
		fmt.Fprintf(out, "  Username: %s\n", connection.Username)
	}
	fmt.Fprintf(out, "  Token ID: %s\n", connection.TokenID)
}

func confirmProfileRemoval(cmd *cobra.Command, yes bool) error {
	if yes {
		return nil
	}
	input := cmd.InOrStdin()
	filer, ok := input.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(filer.Fd())) {
		return errors.New("refusing non-interactive profile removal without --yes")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Proceed? [y/N]: ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return errors.New("profile removal cancelled")
	}
	return nil
}

func removeStoredProfile(store *profile.Store, cfg profile.Config, name, token string) error {
	if err := store.Secrets.Delete(name); err != nil {
		return fmt.Errorf("delete saved API token for profile %q: %w", name, err)
	}
	delete(cfg.Profiles, name)
	if err := store.Save(cfg); err != nil {
		if restoreErr := store.Secrets.Set(name, token); restoreErr != nil {
			return fmt.Errorf("remove profile %q configuration: %w (also could not restore saved token: %v)", name, err, restoreErr)
		}
		return fmt.Errorf("remove profile %q configuration: %w", name, err)
	}
	return nil
}
