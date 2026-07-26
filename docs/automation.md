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

Routine profile commands require the certificate thumbprint saved at login. For a self-signed/private endpoint, bootstrap that pin once with `zbxcli auth login --insecure-skip-tls-verify`; do not place that flag in automation. Replace a certificate only through an intentional `auth login --trust-new-certificate` operation.
