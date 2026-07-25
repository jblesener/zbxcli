package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func writeJSON(cmd *cobra.Command, raw json.RawMessage) error {
	if _, err := cmd.OutOrStdout().Write(raw); err != nil {
		return err
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		_, err := fmt.Fprintln(cmd.OutOrStdout())
		return err
	}
	return nil
}
