package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jblesener/zbxcli/internal/zabbix"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const localMaintenanceTimeLayout = "2006-01-02T15:04"

func newMaintenanceCmd(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "maintenance", Short: "Manage Zabbix maintenance windows"}
	cmd.AddCommand(newMaintenanceCreateCmd(deps), newMaintenanceListCmd(deps), newMaintenanceDeleteCmd(deps))
	return cmd
}

type maintenanceConnectionOptions struct {
	profile  string
	insecure bool
}

func addMaintenanceConnectionFlags(cmd *cobra.Command, opts *maintenanceConnectionOptions) {
	cmd.Flags().StringVar(&opts.profile, "profile", "default", "Profile name")
	cmd.Flags().BoolVar(&opts.insecure, "insecure-skip-tls-verify", false, "Disable TLS certificate verification")
}

type maintenanceCreateOptions struct {
	maintenanceConnectionOptions
	name, description, start, end, timezone string
	hosts, hostIDs, groups, groupIDs        []string
	yes                                     bool
}

func newMaintenanceCreateCmd(deps Dependencies) *cobra.Command {
	var opts maintenanceCreateOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a one-time maintenance window",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMaintenanceCreate(cmd, deps, opts)
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "Maintenance name")
	cmd.Flags().StringVar(&opts.description, "description", "", "Maintenance description")
	cmd.Flags().StringVar(&opts.start, "start", "", "Start time (RFC 3339 or YYYY-MM-DDTHH:MM)")
	cmd.Flags().StringVar(&opts.end, "end", "", "End time (RFC 3339 or YYYY-MM-DDTHH:MM)")
	cmd.Flags().StringVar(&opts.timezone, "timezone", "", "IANA timezone for local timestamps (defaults to local time)")
	cmd.Flags().StringSliceVar(&opts.hosts, "host", nil, "Host technical or visible name (repeatable)")
	cmd.Flags().StringSliceVar(&opts.hostIDs, "host-id", nil, "Host ID (repeatable)")
	cmd.Flags().StringSliceVar(&opts.groups, "host-group", nil, "Host group name (repeatable)")
	cmd.Flags().StringSliceVar(&opts.groupIDs, "host-group-id", nil, "Host group ID (repeatable)")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Skip the confirmation prompt")
	addMaintenanceConnectionFlags(cmd, &opts.maintenanceConnectionOptions)
	return cmd
}

func runMaintenanceCreate(cmd *cobra.Command, deps Dependencies, opts maintenanceCreateOptions) error {
	if strings.TrimSpace(opts.name) == "" {
		return errors.New("--name is required")
	}
	start, err := parseMaintenanceTime(opts.start, opts.timezone)
	if err != nil {
		return fmt.Errorf("parse --start: %w", err)
	}
	end, err := parseMaintenanceTime(opts.end, opts.timezone)
	if err != nil {
		return fmt.Errorf("parse --end: %w", err)
	}
	if !end.After(start) {
		return errors.New("--end must be after --start")
	}
	api, token, err := profileAPI(cmd, deps, opts.profile, opts.insecure, "manage maintenance")
	if err != nil {
		return err
	}
	hostIDs, hostNames, err := resolveMaintenanceHosts(cmd, api, token, opts.hostIDs, opts.hosts)
	if err != nil {
		return err
	}
	groupIDs, groupNames, err := resolveMaintenanceGroups(cmd, api, token, opts.groupIDs, opts.groups)
	if err != nil {
		return err
	}
	if len(hostIDs) == 0 && len(groupIDs) == 0 {
		return errors.New("provide at least one --host, --host-id, --host-group, or --host-group-id")
	}
	writeMaintenancePreview(cmd.ErrOrStderr(), opts.name, opts.description, start, end, hostNames, groupNames)
	if err := confirmMaintenanceMutation(cmd, opts.yes); err != nil {
		return err
	}
	id, err := api.CreateMaintenance(cmd.Context(), token, zabbix.MaintenanceCreate{
		Name: opts.name, Description: opts.description, Start: start, End: end, HostIDs: hostIDs, GroupIDs: groupIDs,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created maintenance %s.\n", id)
	return err
}

func parseMaintenanceTime(raw, zone string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("a timestamp is required")
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		if parsed.Second() != 0 || parsed.Nanosecond() != 0 {
			return time.Time{}, errors.New("timestamps must be precise to a whole minute")
		}
		return parsed, nil
	}
	loc := time.Local
	if strings.TrimSpace(zone) != "" {
		var err error
		loc, err = time.LoadLocation(zone)
		if err != nil {
			return time.Time{}, fmt.Errorf("load timezone %q: %w", zone, err)
		}
	}
	parsed, err := time.ParseInLocation(localMaintenanceTimeLayout, raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("use RFC 3339 or %s: %w", localMaintenanceTimeLayout, err)
	}
	if parsed.In(loc).Format(localMaintenanceTimeLayout) != raw {
		return time.Time{}, errors.New("local timestamp does not exist in the selected timezone")
	}
	for minutes := -180; minutes <= 180; minutes++ {
		if minutes != 0 && parsed.Add(time.Duration(minutes)*time.Minute).In(loc).Format(localMaintenanceTimeLayout) == raw {
			return time.Time{}, errors.New("local timestamp is ambiguous in the selected timezone; use RFC 3339 with an offset")
		}
	}
	return parsed, nil
}

func resolveMaintenanceHosts(cmd *cobra.Command, api *zabbix.Client, token string, ids, names []string) ([]string, []string, error) {
	resolvedIDs, labels := uniqueNonEmpty(ids), make([]string, 0, len(ids)+len(names))
	for _, id := range resolvedIDs {
		labels = append(labels, "ID "+id)
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, nil, errors.New("--host must not be empty")
		}
		matches, err := api.FindHost(cmd.Context(), token, name)
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 {
			return nil, nil, fmt.Errorf("host %q was not found; use --host-id to target it explicitly", name)
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("host %q is ambiguous (%s); use --host-id", name, joinHostMatches(matches))
		}
		resolvedIDs = appendUnique(resolvedIDs, matches[0].ID)
		labels = append(labels, hostLabel(matches[0]))
	}
	return resolvedIDs, uniqueNonEmpty(labels), nil
}

func resolveMaintenanceGroups(cmd *cobra.Command, api *zabbix.Client, token string, ids, names []string) ([]string, []string, error) {
	resolvedIDs, labels := uniqueNonEmpty(ids), make([]string, 0, len(ids)+len(names))
	for _, id := range resolvedIDs {
		labels = append(labels, "ID "+id)
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, nil, errors.New("--host-group must not be empty")
		}
		matches, err := api.FindHostGroup(cmd.Context(), token, name)
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 {
			return nil, nil, fmt.Errorf("host group %q was not found; use --host-group-id to target it explicitly", name)
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("host group %q is ambiguous (%s); use --host-group-id", name, joinGroupMatches(matches))
		}
		resolvedIDs = appendUnique(resolvedIDs, matches[0].ID)
		labels = append(labels, matches[0].Name)
	}
	return resolvedIDs, uniqueNonEmpty(labels), nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func hostLabel(target zabbix.MaintenanceTarget) string {
	if target.Host != "" && target.Name != "" && target.Host != target.Name {
		return target.Host + " (" + target.Name + ")"
	}
	if target.Host != "" {
		return target.Host
	}
	return target.Name
}

func joinHostMatches(matches []zabbix.MaintenanceTarget) string {
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match.ID+": "+hostLabel(match))
	}
	return strings.Join(values, ", ")
}

func joinGroupMatches(matches []zabbix.MaintenanceTarget) string {
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match.ID+": "+match.Name)
	}
	return strings.Join(values, ", ")
}

func writeMaintenancePreview(out io.Writer, name, description string, start, end time.Time, hosts, groups []string) {
	fmt.Fprintln(out, "Maintenance preview:")
	fmt.Fprintf(out, "  Name: %s\n", name)
	fmt.Fprintf(out, "  Start: %s\n", start.Format(time.RFC3339))
	fmt.Fprintf(out, "  End: %s\n", end.Format(time.RFC3339))
	if description != "" {
		fmt.Fprintf(out, "  Description: %s\n", description)
	}
	if len(hosts) > 0 {
		fmt.Fprintf(out, "  Hosts: %s\n", strings.Join(hosts, ", "))
	}
	if len(groups) > 0 {
		fmt.Fprintf(out, "  Host groups: %s\n", strings.Join(groups, ", "))
	}
}

func confirmMaintenanceMutation(cmd *cobra.Command, yes bool) error {
	if yes {
		return nil
	}
	input := cmd.InOrStdin()
	filer, ok := input.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(filer.Fd())) {
		return errors.New("refusing non-interactive mutation without --yes")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Proceed? [y/N]: ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes" {
		return errors.New("maintenance operation cancelled")
	}
	return nil
}

type maintenanceListOptions struct {
	maintenanceConnectionOptions
	limit  int
	output string
}

func newMaintenanceListCmd(deps Dependencies) *cobra.Command {
	var opts maintenanceListOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List maintenance windows",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runMaintenanceList(cmd, deps, opts) },
	}
	cmd.Flags().IntVar(&opts.limit, "limit", defaultQueryLimit, "Maximum number of maintenance windows to return")
	cmd.Flags().StringVar(&opts.output, "output", "table", "Output format: table or json")
	addMaintenanceConnectionFlags(cmd, &opts.maintenanceConnectionOptions)
	return cmd
}

func runMaintenanceList(cmd *cobra.Command, deps Dependencies, opts maintenanceListOptions) error {
	if opts.limit <= 0 {
		return errors.New("--limit must be positive")
	}
	output := strings.ToLower(strings.TrimSpace(opts.output))
	if output != "table" && output != "json" {
		return fmt.Errorf("unsupported --output %q; use table or json", opts.output)
	}
	api, token, err := profileAPI(cmd, deps, opts.profile, opts.insecure, "list maintenance")
	if err != nil {
		return err
	}
	if output == "json" {
		raw, err := api.ListMaintenancesRaw(cmd.Context(), token, opts.limit)
		if err != nil {
			return err
		}
		return writeJSON(cmd, raw)
	}
	records, err := api.ListMaintenances(cmd.Context(), token, opts.limit)
	if err != nil {
		return err
	}
	return writeMaintenanceTable(cmd, records)
}

func writeMaintenanceTable(cmd *cobra.Command, records []zabbix.Maintenance) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "MAINTENANCEID\tNAME\tSTART\tEND\tHOSTS\tHOST GROUPS\tDESCRIPTION"); err != nil {
		return err
	}
	for _, record := range records {
		hosts := make([]string, 0, len(record.Hosts))
		for _, host := range record.Hosts {
			hosts = append(hosts, hostLabel(host))
		}
		groups := make([]string, 0, len(record.Groups))
		for _, group := range record.Groups {
			groups = append(groups, group.Name)
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", record.ID, record.Name, formatMaintenanceEpoch(record.ActiveSince), formatMaintenanceEpoch(record.ActiveTill), strings.Join(hosts, ", "), strings.Join(groups, ", "), record.Description); err != nil {
			return err
		}
	}
	return w.Flush()
}

func formatMaintenanceEpoch(raw string) string {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	return time.Unix(seconds, 0).In(time.Local).Format(time.RFC3339)
}

type maintenanceDeleteOptions struct {
	maintenanceConnectionOptions
	yes bool
}

func newMaintenanceDeleteCmd(deps Dependencies) *cobra.Command {
	var opts maintenanceDeleteOptions
	cmd := &cobra.Command{
		Use:   "delete <maintenance-id> [maintenance-id...]",
		Short: "Delete maintenance windows",
		Args:  cobra.MinimumNArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return runMaintenanceDelete(cmd, deps, opts, args) },
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Skip the confirmation prompt")
	addMaintenanceConnectionFlags(cmd, &opts.maintenanceConnectionOptions)
	return cmd
}

func runMaintenanceDelete(cmd *cobra.Command, deps Dependencies, opts maintenanceDeleteOptions, ids []string) error {
	ids = uniqueNonEmpty(ids)
	if len(ids) == 0 {
		return errors.New("at least one maintenance ID is required")
	}
	api, token, err := profileAPI(cmd, deps, opts.profile, opts.insecure, "manage maintenance")
	if err != nil {
		return err
	}
	sort.Strings(ids)
	fmt.Fprintf(cmd.ErrOrStderr(), "Delete maintenance IDs: %s\n", strings.Join(ids, ", "))
	if err := confirmMaintenanceMutation(cmd, opts.yes); err != nil {
		return err
	}
	if err := api.DeleteMaintenances(cmd.Context(), token, ids); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted maintenance %s.\n", strings.Join(ids, ", "))
	return err
}
