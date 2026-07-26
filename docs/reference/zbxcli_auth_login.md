---
layout: reference
title: "zbxcli auth login"
---

## zbxcli auth login

Create and store a Zabbix API token

```
zbxcli auth login [flags]
```

### Options

```
      --expires-in string          Token lifetime (for example 90d, 24h, or never) (default "90d")
  -h, --help                       help for login
      --insecure-skip-tls-verify   Bootstrap trust for an unverified TLS certificate
      --profile string             Profile name (default "default")
      --show-token                 Print the generated token to stdout
      --token-name string          Name for the Zabbix API token
      --trust-new-certificate      Replace this profile's saved TLS certificate thumbprint
      --url string                 Zabbix frontend or API URL
      --username string            Zabbix username
```

### SEE ALSO

* [zbxcli auth](zbxcli_auth.html)	 - Manage Zabbix authentication

