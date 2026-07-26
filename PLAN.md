# zbxcli functionality roadmap

## Direction

zbxcli is a safe, operator-focused Zabbix CLI. It retains flexible read queries
for automation while adding first-class workflows for frequent operational
tasks.

## First milestone: one-time maintenance

- Add safe create, list, and delete commands for host and host-group
  maintenance.
- Resolve friendly target names exactly and require explicit IDs if lookup is
  missing or ambiguous.
- Support RFC 3339 and local datetime inputs, with an optional IANA timezone.
  Continue data collection while suppressing maintenance problems.
- Preview all mutations and require interactive confirmation or `--yes` for
  automation.

## Next milestones

1. Profile lifecycle: inspect expiry, rotate credentials, and remove profiles.
2. Operator reads: host status, active problems, events, history, and trends.
3. Safe mutations: host enable/disable and problem acknowledgement using the
   same resolution and confirmation model.
4. Recurring maintenance, updates, descriptions/tags, and richer filtering.
5. Consistent help, JSON contracts, shell completion, and end-to-end docs.

## Quality bar

Every workflow needs HTTP-mocked API coverage, version-specific authorization
coverage, clear permission errors, JSON output for automation where applicable,
and passing `go test ./...` plus `go build ./...` in CI.
