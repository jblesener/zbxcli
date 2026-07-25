package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type resourceTableColumn struct {
	header string
	field  string
	format func(string) string
	value  func(map[string]any) string
}

type resourceTableSpec struct {
	columns []resourceTableColumn
}

var resourceTableSpecs = map[string]resourceTableSpec{
	"host": {columns: []resourceTableColumn{
		column("HOSTID", "hostid"), column("HOST", "host"), column("NAME", "name"), formattedColumn("STATUS", "status", hostStatus),
	}},
	"host-group": {columns: []resourceTableColumn{
		column("GROUPID", "groupid"), column("NAME", "name"),
	}},
	"host-interface": {columns: []resourceTableColumn{
		column("INTERFACEID", "interfaceid"), column("HOSTID", "hostid"), formattedColumn("TYPE", "type", interfaceType), {header: "ADDRESS", value: interfaceAddress}, column("PORT", "port"), formattedColumn("AVAILABILITY", "available", interfaceAvailability),
	}},
	"template": {columns: []resourceTableColumn{
		column("TEMPLATEID", "templateid"), column("HOST", "host"), column("NAME", "name"), formattedColumn("STATUS", "status", enabledStatus),
	}},
	"item":              itemColumns(true),
	"discovery-rule":    itemColumns(true),
	"item-prototype":    itemColumns(false),
	"trigger":           triggerColumns(),
	"trigger-prototype": triggerColumns(),
	"graph":             graphColumns(),
	"graph-prototype":   graphColumns(),
	"host-prototype": {columns: []resourceTableColumn{
		column("HOSTID", "hostid"), column("DISCOVERYID", "discoveryid"), column("HOST", "host"), column("NAME", "name"), formattedColumn("STATUS", "status", enabledStatus),
	}},
	"web-scenario": {columns: []resourceTableColumn{
		column("HTTPTESTID", "httptestid"), column("HOSTID", "hostid"), column("NAME", "name"), formattedColumn("STATUS", "status", enabledStatus), column("DELAY", "delay"), column("RETRIES", "retries"),
	}},
}

func column(header, field string) resourceTableColumn {
	return resourceTableColumn{header: header, field: field}
}

func formattedColumn(header, field string, format func(string) string) resourceTableColumn {
	return resourceTableColumn{header: header, field: field, format: format}
}

func itemColumns(includeHostID bool) resourceTableSpec {
	columns := []resourceTableColumn{column("ITEMID", "itemid")}
	if includeHostID {
		columns = append(columns, column("HOSTID", "hostid"))
	}
	columns = append(columns,
		column("NAME", "name"),
		column("KEY", "key_"),
		formattedColumn("TYPE", "type", itemType),
		formattedColumn("STATUS", "status", enabledStatus),
		formattedColumn("STATE", "state", itemState),
	)
	return resourceTableSpec{columns: columns}
}

func triggerColumns() resourceTableSpec {
	return resourceTableSpec{columns: []resourceTableColumn{
		column("TRIGGERID", "triggerid"),
		column("DESCRIPTION", "description"),
		formattedColumn("PRIORITY", "priority", triggerPriority),
		formattedColumn("STATUS", "status", enabledStatus),
		formattedColumn("VALUE", "value", triggerValue),
	}}
}

func graphColumns() resourceTableSpec {
	return resourceTableSpec{columns: []resourceTableColumn{
		column("GRAPHID", "graphid"), column("NAME", "name"), column("WIDTH", "width"), column("HEIGHT", "height"),
	}}
}

func writeResourceTable(cmd *cobra.Command, resource string, raw json.RawMessage) error {
	resource = strings.ToLower(strings.TrimSpace(resource))
	spec, ok := resourceTableSpecs[resource]
	if !ok {
		return fmt.Errorf("no table renderer for resource %q", resource)
	}

	var records []map[string]any
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("decode %s query result as a record list: %w", resource, err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	for index, col := range spec.columns {
		if index > 0 {
			if _, err := fmt.Fprint(w, "\t"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, col.header); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for _, record := range records {
		for index, col := range spec.columns {
			if index > 0 {
				if _, err := fmt.Fprint(w, "\t"); err != nil {
					return err
				}
			}
			value := recordValue(record, col.field)
			if col.value != nil {
				value = col.value(record)
			} else if col.format != nil {
				value = col.format(value)
			}
			if _, err := fmt.Fprint(w, value); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return w.Flush()
}

func recordValue(record map[string]any, field string) string {
	value, ok := record[field]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%v", typed)
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		return fmt.Sprint(typed)
	}
}

func interfaceAddress(record map[string]any) string {
	if recordValue(record, "useip") == "1" {
		return recordValue(record, "ip")
	}
	return recordValue(record, "dns")
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

func enabledStatus(status string) string {
	switch status {
	case "0":
		return "Enabled"
	case "1":
		return "Disabled"
	default:
		return status
	}
}

func interfaceType(kind string) string {
	if label, ok := map[string]string{"1": "Agent", "2": "SNMP", "3": "IPMI", "4": "JMX"}[kind]; ok {
		return label
	}
	return kind
}

func interfaceAvailability(availability string) string {
	if label, ok := map[string]string{"0": "Unknown", "1": "Available", "2": "Unavailable"}[availability]; ok {
		return label
	}
	return availability
}

func itemType(kind string) string {
	if label, ok := map[string]string{
		"0": "Zabbix agent", "2": "Zabbix trapper", "3": "Simple check", "5": "Internal check", "7": "Zabbix agent active", "9": "Web item", "10": "External check", "11": "Database monitor", "12": "IPMI agent", "13": "SSH agent", "14": "TELNET agent", "15": "Calculated", "16": "JMX agent", "17": "SNMP trap", "18": "Dependent", "19": "HTTP agent", "20": "SNMP agent", "21": "Script",
	}[kind]; ok {
		return label
	}
	return kind
}

func itemState(state string) string {
	if label, ok := map[string]string{"0": "Normal", "1": "Not supported"}[state]; ok {
		return label
	}
	return state
}

func triggerPriority(priority string) string {
	if label, ok := map[string]string{"0": "Not classified", "1": "Information", "2": "Warning", "3": "Average", "4": "High", "5": "Disaster"}[priority]; ok {
		return label
	}
	return priority
}

func triggerValue(value string) string {
	if label, ok := map[string]string{"0": "OK", "1": "Problem"}[value]; ok {
		return label
	}
	return value
}
