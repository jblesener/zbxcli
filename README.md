# zbxcli

`zbxcli` is a command-line client for Zabbix. Its first feature creates and securely stores an API token for a Zabbix user.

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
