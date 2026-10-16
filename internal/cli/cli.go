// Package cli holds the cali command tree (cobra, ADR 0003). Output is plain
// text; styled output comes later.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/dirs"
	"github.com/rojcarranza/cali/internal/install"
)

// version is printed by --version.
var version = "dev"

// allowFileScheme admits file:// sources. Only tests set it.
var allowFileScheme = false

// Execute runs cali with args (without the program name) and returns the exit code.
func Execute(args []string, out, errw io.Writer) int {
	root := newRoot(out)
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errw)
	if err := root.ExecuteContext(context.Background()); err != nil {
		if !errors.Is(err, errFindings) {
			_, _ = fmt.Fprintln(errw, "cali: "+err.Error())
		}
		return 1
	}
	return 0
}

func newRoot(out io.Writer) *cobra.Command {
	var global bool
	root := &cobra.Command{
		Use:           "cali",
		Short:         "Install and manage AI agent skills from git repositories",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("cali {{.Version}}\n")
	root.PersistentFlags().BoolVarP(&global, "global", "g", false, "use the global scope (your home folder) instead of the project")

	root.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create cali.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := deps(out)
			if err != nil {
				return err
			}
			p, err := install.Init(d, global)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "created %s\n", p)
			return err
		},
	})

	var req install.AddRequest
	add := &cobra.Command{
		Use:   "add <spec>",
		Short: "Install a skill from a git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deps(out)
			if err != nil {
				return err
			}
			req.Spec = args[0]
			req.Global = global
			_, err = install.Add(cmd.Context(), d, req)
			return err
		},
	}
	add.Flags().StringVar(&req.Skill, "skill", "", "install the skill with this name when the repo has several")
	add.Flags().BoolVar(&req.All, "all", false, "install every skill in the repo")
	add.Flags().StringSliceVar(&req.Targets, "target", nil, "target to install into: claude or copilot (repeatable)")
	add.Flags().BoolVar(&req.Adopt, "adopt", false, "take over an existing folder that is not managed by Cali, as is")
	root.AddCommand(add, newListCmd(out, &global), newShowCmd(out, &global), newStatusCmd(out, &global),
		newSyncCmd(out, &global), newRemoveCmd(out, &global), newFindCmd(out, &global))
	return root
}

func deps(out io.Writer) (install.Deps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return install.Deps{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return install.Deps{}, err
	}
	return install.Deps{
		Dirs:            dirs.Resolve(runtime.GOOS, os.Getenv, home),
		Stdout:          out,
		Getenv:          os.Getenv,
		GhToken:         ghToken,
		AllowFileScheme: allowFileScheme,
		Cwd:             cwd,
	}, nil
}

// ghToken asks the GitHub CLI for a token; any failure means no token.
func ghToken() (string, error) {
	b, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
