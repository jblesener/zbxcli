---
layout: default
title: Authenticate with Zabbix
---

# Authenticate with Zabbix

Create a profile and Zabbix API token:

```sh
zbxcli auth login --url https://zabbix.example/zabbix --username Admin
```

The password is requested from the interactive terminal. zbxcli stores the resulting token in the operating-system credential manager and saves only connection metadata under the user configuration directory.

## Profiles and expiry

Use a profile for each Zabbix connection and set a token lifetime that matches your policy:

```sh
zbxcli auth login --profile production --url https://zabbix.example --username Admin --expires-in 30d
```

The default lifetime is 90 days. Use `--expires-in never` only when your token policy permits a non-expiring token. Re-running login for a profile replaces the local credential and attempts to revoke the prior token.

## Script access and TLS

`auth token show --profile production` writes only the token and trailing newline. Treat it as a secret and keep it out of shell history and logs.

TLS certificates are verified by default. `--insecure-skip-tls-verify` is intended only for deliberate testing or trusted private deployments.
