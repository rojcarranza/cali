package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/install"
)

func newRemoveCmd(out io.Writer, global *bool) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Back up and remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deps(out)
			if err != nil {
				return err
			}
			d.Global = *global
			return install.Remove(cmd.Context(), d, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove even if the folder has local edits (a backup is kept)")
	return cmd
}
