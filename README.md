# zbxcli

`zbxcli` is a command-line client for Zabbix. Its first feature creates and securely stores an API token for a Zabbix user.

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

## Query hosts

List the hosts visible to a stored profile:

```sh
zbxcli host get --profile production
```

Pass an optional query to search the technical and visible host names case-insensitively. Results are limited to 100 by default; change that with `--limit`.

```sh
zbxcli host get web --profile production --limit 25
```

The default output is a table. Use `--output json` to write the complete host records returned by Zabbix for scripting:

```sh
zbxcli host get web --profile production --output json
```

## Query data-collection resources

Query the remaining supported data-collection models as complete JSON records:

```sh
zbxcli get item --profile production --params '{"search":{"name":"CPU"},"searchByAny":true}'
```

The supported resources are `host`, `host-group`, `host-interface`, `template`, `item`, `trigger`, `graph`, `discovery-rule`, `item-prototype`, `trigger-prototype`, `graph-prototype`, `host-prototype`, and `web-scenario`. Each maps to the corresponding read-only Zabbix `*.get` API method.

Pass model-specific Zabbix API parameters in `--params` as a JSON object. `zbxcli` always requests complete records and limits results to 100 by default; use `--limit` to change that. The `output` and `limit` API parameters are managed by the command and must not appear in `--params`.
