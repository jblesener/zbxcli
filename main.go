package main

import (
	"os"

	"github.com/jblesener/zbxcli/internal/cli"
)

func main() {
	if err := cli.NewRootCmd(cli.DefaultDependencies()).Execute(); err != nil {
		os.Exit(1)
	}
}
