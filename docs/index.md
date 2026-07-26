---
layout: default
title: Zabbix from the terminal
description: Install, authenticate, query, and manage Zabbix maintenance safely with zbxcli.
---

<p class="eyebrow">Zabbix command-line client</p>

# Zabbix operations with safe defaults

<p class="lead">zbxcli creates and stores Zabbix API tokens securely, gives scripts clean JSON output, and brings deliberate confirmations to maintenance operations.</p>

```sh
brew tap jblesener/tools
brew install zbxcli
zbxcli auth login --url https://zabbix.example --username Admin
zbxcli get host web
```

<div class="callout">
Connection metadata is stored separately from the API token. TLS verification is enabled by default, and maintenance mutations require a preview plus confirmation.
</div>

## Start here

- [Install zbxcli](installation.html) using Homebrew, Go, or a release archive.
- [Create and use a Zabbix token](authentication.html) with profile-aware credentials.
- [Query data-collection resources](queries.html) in a terminal table or JSON.
- [Manage one-time maintenance](maintenance.html) with explicit target resolution.
- Review [automation and safety guidance](automation.html) before using `--yes` in CI.

## What it supports

zbxcli supports Zabbix 6.0 and newer. It reads common data-collection resources through their `*.get` API methods and manages one-time host or host-group maintenance windows. The generated [command reference](reference/) contains every current command and flag.
