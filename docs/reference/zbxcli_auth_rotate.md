---
layout: reference
title: "zbxcli auth rotate"
---

## zbxcli auth rotate

Rotate a saved profile's API token

```
zbxcli auth rotate [flags]
```

### Options

```
      --expires-in string          Token lifetime (for example 90d, 24h, or never) (default "90d")
  -h, --help                       help for rotate
      --insecure-skip-tls-verify   Disable TLS certificate verification
      --profile string             Profile name (default "default")
      --show-token                 Print the generated token to stdout
      --token-name string          Name for the Zabbix API token
      --username string            Zabbix username (defaults to the saved username)
```

### SEE ALSO

* [zbxcli auth](zbxcli_auth.html)	 - Manage Zabbix authentication

