---
layout: default
title: Query Zabbix resources
---

# Query Zabbix resources

zbxcli queries supported data-collection resources through their read-only Zabbix `*.get` APIs. The result is a compact table by default:

```sh
zbxcli get item --profile production --params '{"search":{"name":"CPU"},"searchByAny":true}'
```

Supported resources are `host`, `host-group`, `host-interface`, `template`, `item`, `trigger`, `graph`, `discovery-rule`, `item-prototype`, `trigger-prototype`, `graph-prototype`, `host-prototype`, and `web-scenario`.

## Find a host

Only `host` accepts the optional friendly query. It searches technical and visible host names case-insensitively:

```sh
zbxcli get host web --profile production --limit 25
```

## API parameters and JSON

Pass model-specific API parameters as one JSON object. zbxcli always controls the API `output` and `limit` values, so they cannot appear in `--params`.

```sh
zbxcli get item --profile production --output json
```

The default result limit is 100. Use `--limit` to change it. When using the host shortcut, do not also set `search` or `searchByAny` in `--params`.
