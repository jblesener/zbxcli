# zbxcli

`zbxcli` is a command-line client for Zabbix. Its first feature creates and securely stores an API token for a Zabbix user.

Full installation, workflow, and command-reference documentation is available at [jblesener.github.io/zbxcli](https://jblesener.github.io/zbxcli/).

## Development

Commits follow the [Conventional Commits](https://www.conventionalcommits.org/) format. Install the Node development tools with `npm install` to enable the local commit-message hook. For example, use `feat: add host search` for a feature or `fix: handle expired tokens` for a bug fix.

Merges to `main` are released automatically: `feat` commits produce minor releases, `fix` and `perf` commits produce patch releases, and breaking changes produce major releases. Other valid commit types do not create a release.

## Authenticate

```sh
zbxcli auth login --url https://zabbix.example/zabbix --username Admin
```

The password is requested from an interactive terminal. The resulting API token is kept in the operating system credential manager; connection metadata is stored in the user configuration directory under `zbxcli/profiles.json`.

Use a separate profile for each Zabbix connection:

```sh
zbxcli auth login --profile production --url https://zabbix.example --username Admin --expires-in 30d
```

`--show-token` prints the newly generated token to standard output. Treat that output as a secret. API tokens expire after 90 days by default; use `--expires-in never` only when your token policy permits it.

To write a saved token for use in a script, run:

```sh
zbxcli auth token show --profile production
```

This command writes only the token and a trailing newline to standard output. Treat its output as a secret.

## Manage profiles

Inspect saved connection metadata and token expiry without reading token values
from the operating-system credential manager:

```sh
zbxcli auth profile list
zbxcli auth profile list --output json
```

The list is sorted by profile name and marks tokens as `active`, `expired`, or
`never`. JSON output contains profile metadata, including the token ID, but
never the token value.

Rotate a profile's API token without changing its endpoint. zbxcli creates and
stores the new token before revoking the old one:

```sh
zbxcli auth rotate --profile production --expires-in 90d
```

The username saved by `auth login` is used by default. Profiles created by older
zbxcli versions prompt for a username; pass `--username` when rotating in a
non-interactive environment. `--show-token` has the same secret-handling
implications as it does for `auth login`.

To permanently retire a profile, revoke its saved token on Zabbix and remove
the local metadata and keychain entry. The command previews the target and
requires confirmation; use `--yes` in automation:

```sh
zbxcli auth remove production
zbxcli auth remove production --yes
```

If remote revocation fails, zbxcli leaves the local profile and token in place
so the operation can be retried.

Zabbix 6.0 and newer are supported. TLS certificates are verified by default; `--insecure-skip-tls-verify` is available only for deliberate testing or private deployments.

## Query data-collection resources

Query any supported data-collection resource as a human-friendly table:

```sh
zbxcli get item --profile production --params '{"search":{"name":"CPU"},"searchByAny":true}'
```

The supported resources are `host`, `host-group`, `host-interface`, `template`, `item`, `trigger`, `graph`, `discovery-rule`, `item-prototype`, `trigger-prototype`, `graph-prototype`, `host-prototype`, and `web-scenario`. Each maps to the corresponding read-only Zabbix `*.get` API method.

For hosts, an optional query searches technical and visible names case-insensitively:

```sh
zbxcli get host web --profile production --limit 25
```

Use `--output json` to write the complete API records for scripting:

```sh
zbxcli get item --profile production --output json
```

Pass model-specific Zabbix API parameters in `--params` as a JSON object. `zbxcli` always requests complete records and limits results to 100 by default; use `--limit` to change that. The `output` and `limit` API parameters are managed by the command and must not appear in `--params`. When using the host query shortcut, do not also set `search` or `searchByAny` in `--params`.

## Manage maintenance windows

Create a one-time maintenance window for exact host or host-group names. Data
collection continues, while problems for the selected targets are suppressed.

```sh
zbxcli maintenance create \
  --profile production \
  --name 'Database upgrade' \
  --start 2026-08-01T01:00:00+09:00 \
  --end 2026-08-01T03:00:00+09:00 \
  --host-group Databases
```

Timestamps may be RFC 3339 or local `YYYY-MM-DDTHH:MM` values. Local values
use the machine timezone by default; pass `--timezone Asia/Tokyo` to make the
zone explicit. Times must fall on whole minutes. Names are resolved exactly
against a host's technical or visible name and a host group's name. A missing
or ambiguous name is rejected; use repeatable `--host-id` or `--host-group-id`
for script-safe targeting. Hosts and groups may be combined.

Before creating or deleting a window, zbxcli prints a preview and asks for
confirmation on an interactive terminal. Use `--yes` in scripts and CI:

```sh
zbxcli maintenance create --name 'Patch window' \
  --start 2026-08-01T01:00:00Z --end 2026-08-01T02:00:00Z \
  --host-id 10105 --yes
zbxcli maintenance list --profile production
zbxcli maintenance delete 42 --profile production --yes
```

Use `maintenance list --output json` for the API records. Managing maintenance
requires a Zabbix Admin or Super admin token with permission to use the
maintenance API.

## License

This project is licensed under the [MIT License](LICENSE).
Copyright © 2026 John Blesener.
