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
