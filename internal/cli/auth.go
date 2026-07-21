package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jblesener/zbxcli/internal/profile"
	"github.com/jblesener/zbxcli/internal/zabbix"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAuthCmd(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Zabbix authentication"}
	cmd.AddCommand(newLoginCmd(deps))
	return cmd
}

func newLoginCmd(deps Dependencies) *cobra.Command {
	var endpoint, username, profileName, expiresIn, tokenName string
	var showToken, insecure bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Create and store a Zabbix API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, deps, loginOptions{endpoint: endpoint, username: username, profile: profileName, expiresIn: expiresIn, tokenName: tokenName, showToken: showToken, insecure: insecure})
		},
	}
	cmd.Flags().StringVar(&endpoint, "url", "", "Zabbix frontend or API URL")
	cmd.Flags().StringVar(&username, "username", "", "Zabbix username")
	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name")
	cmd.Flags().StringVar(&expiresIn, "expires-in", "90d", "Token lifetime (for example 90d, 24h, or never)")
	cmd.Flags().StringVar(&tokenName, "token-name", "", "Name for the Zabbix API token")
	cmd.Flags().BoolVar(&showToken, "show-token", false, "Print the generated token to stdout")
	cmd.Flags().BoolVar(&insecure, "insecure-skip-tls-verify", false, "Disable TLS certificate verification")
	return cmd
}

type loginOptions struct {
	endpoint, username, profile, expiresIn, tokenName string
	showToken, insecure                               bool
}

func runLogin(cmd *cobra.Command, deps Dependencies, opts loginOptions) error {
	if deps.Store == nil || deps.Store.Secrets == nil || deps.Password == nil {
		return errors.New("zbxcli login dependencies are not configured")
	}
	if err := profile.ValidateName(opts.profile); err != nil {
		return err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	var err error
	if strings.TrimSpace(opts.endpoint) == "" {
		opts.endpoint, err = prompt(reader, cmd.ErrOrStderr(), "Zabbix URL")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.username) == "" {
		opts.username, err = prompt(reader, cmd.ErrOrStderr(), "Username")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(opts.username) == "" {
		return errors.New("Zabbix username is required")
	}
	expiresAt, err := parseExpiry(opts.expiresIn, time.Now())
	if err != nil {
		return err
	}
	api, err := zabbix.New(opts.endpoint, opts.insecure)
	if err != nil {
		return err
	}
	version, err := api.Discover(cmd.Context())
	if err != nil {
		return err
	}
	password, err := deps.Password(cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New("password is required")
	}
	session, err := api.Login(cmd.Context(), opts.username, password)
	if err != nil {
		return err
	}
	defer func() {
		if logoutErr := api.Logout(context.Background(), session); logoutErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not close temporary Zabbix session: %v\n", logoutErr)
		}
	}()
	if opts.tokenName == "" {
		opts.tokenName = fmt.Sprintf("zbxcli/%s/%s", opts.profile, time.Now().UTC().Format("20060102T150405Z"))
	}
	issued, err := api.CreateToken(cmd.Context(), session, opts.tokenName, expiresAt)
	if err != nil {
		if issued.ID != "" {
			cleanupToken(cmd, api, session, issued.ID)
		}
		return err
	}
	endpoint, _ := zabbix.NormalizeEndpoint(opts.endpoint)
	previousTokenID, err := persistProfile(deps.Store, opts.profile, profile.Profile{URL: endpoint, APIVersion: version.Raw, TokenID: issued.ID, ExpiresAt: issued.ExpiresAt}, issued.Value)
	if err != nil {
		cleanupToken(cmd, api, session, issued.ID)
		return err
	}
	if previousTokenID != "" {
		if err := api.DeleteToken(context.Background(), session, previousTokenID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not revoke previous token %s: %v\n", previousTokenID, err)
		}
	}
	if opts.showToken {
		fmt.Fprintln(cmd.OutOrStdout(), issued.Value)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Authenticated profile %q against Zabbix API %s.\n", opts.profile, version.Raw)
	}
	return nil
}

func cleanupToken(cmd *cobra.Command, api *zabbix.Client, session, id string) {
	if err := api.DeleteToken(context.Background(), session, id); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not delete newly-created token %s: %v\n", id, err)
	}
}

func persistProfile(store *profile.Store, name string, next profile.Profile, token string) (string, error) {
	cfg, err := store.Load()
	if err != nil {
		return "", err
	}
	previous, replacing := cfg.Profiles[name]
	var previousToken string
	if replacing {
		previousToken, err = store.Secrets.Get(name)
		if err != nil {
			return "", fmt.Errorf("read existing profile token: %w", err)
		}
	}
	if err := store.Secrets.Set(name, token); err != nil {
		return "", fmt.Errorf("store API token securely: %w", err)
	}
	cfg.Profiles[name] = next
	if err := store.Save(cfg); err != nil {
		if replacing {
			_ = store.Secrets.Set(name, previousToken)
		} else {
			_ = store.Secrets.Delete(name)
		}
		return "", err
	}
	if replacing {
		return previous.TokenID, nil
	}
	return "", nil
}

func parseExpiry(raw string, now time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "never" || raw == "0" {
		return nil, nil
	}
	if strings.HasSuffix(raw, "d") {
		days := strings.TrimSuffix(raw, "d")
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err != nil || n <= 0 {
			return nil, errors.New("--expires-in must be a positive duration or never")
		}
		expires := now.Add(time.Duration(n) * 24 * time.Hour)
		return &expires, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return nil, errors.New("--expires-in must be a positive duration or never")
	}
	expires := now.Add(d)
	return &expires, nil
}

func prompt(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprintf(out, "%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func terminalPassword(out io.Writer) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("password prompt requires an interactive terminal")
	}
	fmt.Fprint(out, "Password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(password), nil
}
