---
layout: default
title: Automation and safety
---

# Automation and safety

## Prefer JSON for programs

Tables are intended for operators. Add `--output json` when another program consumes a query or maintenance listing:

```sh
zbxcli get host --profile production --output json
zbxcli maintenance list --profile production --output json
```

## Make the connection and target explicit

Use `--profile NAME` in scripts. For maintenance, use `--host-id` and `--host-group-id` rather than friendly names where repeatability matters. zbxcli rejects non-interactive maintenance mutations unless `--yes` is set.

## Keep tokens out of logs

Only `auth login --show-token` and `auth token show` intentionally write a token. Do not enable shell tracing or store their output in CI logs. Use your secret manager to provide tokens to downstream programs instead.

## Preserve TLS verification

Keep TLS verification enabled for routine operation. If a private test deployment requires `--insecure-skip-tls-verify`, scope it to that command and profile rather than making it a default in automation.
