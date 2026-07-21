package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/jblesener/zbxcli/internal/zabbix"
	"github.com/spf13/cobra"
)

func newHostCmd(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "Query Zabbix hosts"}
	cmd.AddCommand(newHostGetCmd(deps))
	return cmd
}

func newHostGetCmd(deps Dependencies) *cobra.Command {
	var profileName, output string
	var limit int
	var insecure bool
	cmd := &cobra.Command{
		Use:   "get [query]",
		Short: "List hosts visible to a profile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			return runHostGet(cmd, deps, hostGetOptions{profile: profileName, query: query, limit: limit, output: output, insecure: insecure})
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "default", "Profile name")
	cmd.Flags().IntVar(&limit, "limit", defaultQueryLimit, "Maximum number of hosts to return")
	cmd.Flags().StringVar(&output, "output", "table", "Output format: table or json")
	cmd.Flags().BoolVar(&insecure, "insecure-skip-tls-verify", false, "Disable TLS certificate verification")
	return cmd
}

type hostGetOptions struct {
	profile, query, output string
	limit                  int
	insecure               bool
}

func runHostGet(cmd *cobra.Command, deps Dependencies, opts hostGetOptions) error {
	if opts.limit <= 0 {
		return errors.New("--limit must be positive")
	}
	opts.output = strings.ToLower(strings.TrimSpace(opts.output))
	if opts.output != "table" && opts.output != "json" {
		return fmt.Errorf("unsupported --output %q; use table or json", opts.output)
	}

	api, token, err := profileAPI(cmd, deps, opts.profile, opts.insecure, "host query")
	if err != nil {
		return err
	}
	raw, err := api.GetHosts(cmd.Context(), token, zabbix.HostQuery{Search: opts.query, Limit: opts.limit})
	if err != nil {
		return err
	}
	if opts.output == "json" {
		return writeJSON(cmd, raw)
	}
	return writeHostTable(cmd, raw)
}

func writeJSON(cmd *cobra.Command, raw json.RawMessage) error {
	if _, err := cmd.OutOrStdout().Write(raw); err != nil {
		return err
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		_, err := fmt.Fprintln(cmd.OutOrStdout())
		return err
	}
	return nil
}

type hostRecord struct {
	ID     string `json:"hostid"`
	Host   string `json:"host"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func writeHostTable(cmd *cobra.Command, raw json.RawMessage) error {
	var hosts []hostRecord
	if err := json.Unmarshal(raw, &hosts); err != nil {
		return fmt.Errorf("decode host query result: %w", err)
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "HOSTID\tHOST\tNAME\tSTATUS"); err != nil {
		return err
	}
	for _, host := range hosts {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", host.ID, host.Host, host.Name, hostStatus(host.Status)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func hostStatus(status string) string {
	switch status {
	case "0":
		return "Monitored"
	case "1":
		return "Disabled"
	default:
		return status
	}
}
