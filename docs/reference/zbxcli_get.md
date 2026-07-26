---
layout: reference
title: "zbxcli get"
---

## zbxcli get

Query a Zabbix data-collection resource

```
zbxcli get <resource> [query] [flags]
```

### Options

```
  -h, --help                       help for get
      --insecure-skip-tls-verify   Disable TLS certificate verification
      --limit int                  Maximum number of records to return (default 100)
      --output string              Output format: table or json (default "table")
      --params string              JSON object of resource-specific Zabbix API get parameters (default "{}")
      --profile string             Profile name (default "default")
```

### SEE ALSO

* [zbxcli](zbxcli.html)	 - A command-line client for Zabbix

