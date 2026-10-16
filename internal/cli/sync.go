package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/install"
)

func newSyncCmd(out io.Writer, global *bool) *cobra.Command {
	var frozen bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Install every skill in cali.lock; lock manifest skills that are not locked yet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := deps(out)
			if err != nil {
				return err
			}
			d.Global = *global
			_, err = install.Sync(cmd.Context(), d, frozen)
			return err
		},
	}
	cmd.Flags().BoolVar(&frozen, "frozen", false, "fail instead of changing cali.lock")
	return cmd
}
