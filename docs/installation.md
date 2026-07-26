---
layout: default
title: Install zbxcli
---

# Install zbxcli

## Homebrew

Homebrew is the quickest supported installation path on macOS and Linux.

```sh
brew tap jblesener/tools
brew install zbxcli
```

## Go

With Go 1.26 or newer installed, install the latest release directly:

```sh
go install github.com/jblesener/zbxcli@latest
```

Ensure Go's binary directory is on your `PATH`, then verify the installation:

```sh
zbxcli --help
```

## Release archives

[GitHub Releases](https://github.com/jblesener/zbxcli/releases) contain prebuilt archives for macOS, Linux, and Windows on supported architectures. Download the matching archive, verify its checksum when one is published, and place the extracted binary on your `PATH`.

## Build from source

```sh
git clone https://github.com/jblesener/zbxcli.git
cd zbxcli
go build ./...
```
