---
layout: reference
title: "zbxcli maintenance create"
---

## zbxcli maintenance create

Create a one-time maintenance window

```
zbxcli maintenance create [flags]
```

### Options

```
      --description string      Maintenance description
      --end string              End time (RFC 3339 or YYYY-MM-DDTHH:MM)
  -h, --help                    help for create
      --host strings            Host technical or visible name (repeatable)
      --host-group strings      Host group name (repeatable)
      --host-group-id strings   Host group ID (repeatable)
      --host-id strings         Host ID (repeatable)
      --name string             Maintenance name
      --profile string          Profile name (default "default")
      --start string            Start time (RFC 3339 or YYYY-MM-DDTHH:MM)
      --timezone string         IANA timezone for local timestamps (defaults to local time)
      --yes                     Skip the confirmation prompt
```

### SEE ALSO

* [zbxcli maintenance](zbxcli_maintenance.html)	 - Manage Zabbix maintenance windows

