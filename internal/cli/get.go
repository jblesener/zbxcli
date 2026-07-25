package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jblesener/zbxcli/internal/profile"
	"github.com/jblesener/zbxcli/internal/zabbix"
	"github.com/spf13/cobra"
)

const defaultQueryLimit = 100

func newGetCmd(deps Dependencies) *cobra.Command {
	var profileName, params, output string
	var limit int
	var insecure bool
	cmd := &cobra.Command{
		Use:   "get <resource> [query]",
		Short: "Query a Zabbix data-collection resource",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) == 2 {
				query = args[1]
			}
			return runGet(cmd, deps, getOptions{profile: profileName, resource: args[0], query: query, params: params, limit: limit, output: output, insecure: insecure})
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name")
	cmd.Flags().IntVar(&limit, "limit", defaultQueryLimit, "Maximum number of records to return")
	cmd.Flags().StringVar(&params, "params", "{}", "JSON object of resource-specific Zabbix API get parameters")
	cmd.Flags().StringVar(&output, "output", "table", "Output format: table or json")
	cmd.Flags().BoolVar(&insecure, "insecure-skip-tls-verify", false, "Disable TLS certificate verification")
	return cmd
}

type getOptions struct {
	profile, resource, query, params, output string
	limit                                    int
	insecure                                 bool
}

func runGet(cmd *cobra.Command, deps Dependencies, opts getOptions) error {
	if opts.limit <= 0 {
		return errors.New("--limit must be positive")
	}
	params, err := parseResourceParams(opts.params)
	if err != nil {
		return err
	}
	resource := strings.ToLower(strings.TrimSpace(opts.resource))
	opts.query = strings.TrimSpace(opts.query)
	if opts.query != "" {
		if resource != "host" {
			return errors.New("an optional query is supported only for the host resource")
		}
		if _, ok := params["search"]; ok {
			return errors.New("--params must not include \"search\" when a host query is provided")
		}
		if _, ok := params["searchByAny"]; ok {
			return errors.New("--params must not include \"searchByAny\" when a host query is provided")
		}
		params["search"] = map[string]string{"host": opts.query, "name": opts.query}
		params["searchByAny"] = true
	}
	opts.output = strings.ToLower(strings.TrimSpace(opts.output))
	if opts.output != "table" && opts.output != "json" {
		return fmt.Errorf("unsupported --output %q; use table or json", opts.output)
	}
	api, token, err := profileAPI(cmd, deps, opts.profile, opts.insecure, "query")
	if err != nil {
		return err
	}
	raw, err := api.GetResource(cmd.Context(), token, resource, zabbix.ResourceQuery{Params: params, Limit: opts.limit})
	if err != nil {
		return err
	}
	if opts.output == "json" {
		return writeJSON(cmd, raw)
	}
	return writeResourceTable(cmd, resource, raw)
}

func parseResourceParams(raw string) (map[string]any, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &params); err != nil || params == nil {
		if err != nil {
			return nil, fmt.Errorf("parse --params JSON object: %w", err)
		}
		return nil, errors.New("--params must be a JSON object")
	}
	for _, key := range []string{"output", "limit"} {
		if _, ok := params[key]; ok {
			return nil, fmt.Errorf("--params must not include %q; use the command defaults or --limit", key)
		}
	}
	return params, nil
}

func profileAPI(cmd *cobra.Command, deps Dependencies, profileName string, insecure bool, action string) (*zabbix.Client, string, error) {
	if deps.Store == nil || deps.Store.Secrets == nil {
		return nil, "", fmt.Errorf("zbxcli %s dependencies are not configured", action)
	}
	if err := profile.ValidateName(profileName); err != nil {
		return nil, "", err
	}
	cfg, err := deps.Store.Load()
	if err != nil {
		return nil, "", err
	}
	connection, ok := cfg.Profiles[profileName]
	if !ok {
		return nil, "", fmt.Errorf("profile %q is not configured; run zbxcli auth login --profile %s", profileName, profileName)
	}
	token, err := deps.Store.Secrets.Get(profileName)
	if err != nil {
		return nil, "", fmt.Errorf("load API token for profile %q: %w", profileName, err)
	}
	api, err := zabbix.New(connection.URL, insecure)
	if err != nil {
		return nil, "", err
	}
	if _, err := api.Discover(cmd.Context()); err != nil {
		return nil, "", err
	}
	return api, token, nil
}
