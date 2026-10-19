package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/install"
)

// interactive and confirm are vars so tests can answer the first-run prompt.
var (
	interactive = func(w io.Writer) bool { return isTTY(w) && isTTY(os.Stdin) } // huh reads stdin
	confirm     = func(q string) (bool, error) {
		ok := true
		err := huh.NewConfirm().Title(q).Value(&ok).Run()
		return ok, err
	}
)

// exists reports whether p exists; an error other than not-exist is returned.
func exists(p string) (bool, error) {
	_, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

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
				if p, err := install.Locate(d, false); err == nil {
					if ok, err := exists(p.Manifest); err != nil {
						return err
					} else if ok {
						d.Manifest = p.Manifest
					}
				}
			}
			loc, err := install.Locate(d, *global)
			if err != nil {
				return err
			}
			haveManifest, err := exists(loc.Manifest)
			if err != nil {
				return err
			}
			haveLock, err := exists(loc.LockPath)
			if err != nil {
				return err
			}
			if !haveManifest && !haveLock {
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
