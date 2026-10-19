package cli

import (
	"fmt"
	"io"
	"os"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/install"
)

// interactive and confirm are vars so tests can answer the first-run prompt.
var (
	interactive = isTTY
	confirm     = func(q string) (bool, error) {
		ok := true
		err := huh.NewConfirm().Title(q).Value(&ok).Run()
		return ok, err
	}
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

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
			// -g in a project installs the project's cali.yaml into your home folder.
			if *global && d.Manifest == "" {
				if p, err := install.Locate(d, false); err == nil && exists(p.Manifest) {
					d.Manifest = p.Manifest
				}
			}
			loc, err := install.Locate(d, *global)
			if err != nil {
				return err
			}
			if !exists(loc.Manifest) && !exists(loc.LockPath) {
				return firstRun(cmd, d, loc.Manifest, frozen)
			}
			_, err = install.Sync(cmd.Context(), d, frozen)
			return err
		},
	}
	cmd.Flags().BoolVar(&frozen, "frozen", false, "fail instead of changing cali.lock")
	return cmd
}

// firstRun handles sync with no cali.yaml: on a terminal it offers to install
// every skill of $CALI_DEFAULT_SOURCE, otherwise it prints how to start.
func firstRun(cmd *cobra.Command, d install.Deps, manifest string, frozen bool) error {
	src := os.Getenv("CALI_DEFAULT_SOURCE")
	if src != "" && !frozen && interactive(d.Stdout) {
		ok, err := confirm(fmt.Sprintf("No %s yet. Install every skill from %s?", manifest, src))
		if err != nil {
			return err
		}
		if ok {
			_, err = install.Add(cmd.Context(), d, install.AddRequest{Spec: src, Global: d.Global})
			return err
		}
	}
	if src == "" {
		src = "<source>"
	}
	if d.Global {
		src = "-g " + src
	}
	_, err := fmt.Fprintf(d.Stdout, "no %s; start with: cali add %s\n", manifest, src)
	return err
}
