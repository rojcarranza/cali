// Package cli holds the cali command tree. Task 8 replaces this stub.
package cli

import (
	"fmt"
	"io"
)

// Execute runs cali with args (without the program name) and returns the exit code.
func Execute(args []string, out, errw io.Writer) int {
	for _, a := range args {
		if a == "--version" {
			if _, err := fmt.Fprintln(out, "cali dev"); err != nil {
				_, _ = fmt.Fprintln(errw, err)
				return 1
			}
			return 0
		}
	}
	return 0
}
