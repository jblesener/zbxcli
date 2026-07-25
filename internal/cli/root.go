package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/jblesener/zbxcli/internal/profile"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Store    *profile.Store
	Password func(io.Writer) (string, error)
}

func DefaultDependencies() Dependencies {
	store, err := profile.Default()
	if err != nil {
		panic(fmt.Sprintf("initialize zbxcli configuration: %v", err))
	}
	return Dependencies{Store: store, Password: terminalPassword}
}

func NewRootCmd(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "zbxcli", Short: "A command-line client for Zabbix", SilenceUsage: true}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.AddCommand(newAuthCmd(deps))
	cmd.AddCommand(newGetCmd(deps))
	return cmd
}
