---
layout: default
title: Manage maintenance windows
---

# Manage maintenance windows

Create a one-time maintenance window for exact host or host-group names. Data collection continues while problems for the selected targets are suppressed.

```sh
zbxcli maintenance create \
  --profile production \
  --name 'Database upgrade' \
  --start 2026-08-01T01:00:00+09:00 \
  --end 2026-08-01T03:00:00+09:00 \
  --host-group Databases
```

Timestamps can use RFC 3339 or local `YYYY-MM-DDTHH:MM` values. Local values use the machine timezone unless `--timezone Asia/Tokyo` is supplied. Times must be precise to a whole minute.

## Target resolution and confirmation

Friendly host and host-group names resolve exactly. Missing or ambiguous names are rejected; use repeatable `--host-id` or `--host-group-id` for script-safe targeting. Hosts and groups can be combined.

Before creation or deletion, zbxcli prints a preview and requires confirmation on an interactive terminal. Use `--yes` only after deliberately selecting the target:

```sh
zbxcli maintenance create --name 'Patch window' \
  --start 2026-08-01T01:00:00Z --end 2026-08-01T02:00:00Z \
  --host-id 10105 --yes
zbxcli maintenance list --profile production
zbxcli maintenance delete 42 --profile production --yes
```

`maintenance list --output json` returns complete API records. Maintenance management requires a Zabbix Admin or Super admin token with permission to use the maintenance API.
