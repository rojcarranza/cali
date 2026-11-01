package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/status"
)

// Agent mode is for AI agents that run cali in a terminal (AXI rules 4-7, 9):
// it never prompts, usage errors exit 2, and text output opens with totals
// and ends with the next command to run.
const (
	agentEnv      = "CALI_AGENT"
	exitUsage     = 2
	agentFlagName = "agent"
)

// agentMode is set for each run by newRoot and the persistent pre-run hook.
var agentMode bool

// ranCommand is set once a command's RunE is about to run, so an error before
// that point (unknown flag, unknown command, wrong argument count) is a usage error.
var ranCommand bool

func agentFromEnv() bool {
	v := os.Getenv(agentEnv)
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func wantsAgent(args []string) bool {
	return agentFromEnv() || slices.Contains(args, "--"+agentFlagName)
}

func agentPreRun(flag *bool) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		ranCommand = true
		agentMode = *flag || agentFromEnv()
		return nil
	}
}

// listTotals is the first line of list in agent mode.
func listTotals(n int) string {
	if n == 0 {
		return "0 skills\nnext: cali add <source>\n"
	}
	return fmt.Sprintf("%d skills\n", n)
}

// statusTotals summarises rows as "N skills: 2 ok, 1 outdated" and returns the next command, if any.
func statusTotals(rows []status.Row) (line, next string) {
	skills := map[string]bool{}
	counts := map[status.State]int{}
	for _, r := range rows {
		skills[r.Skill] = true
		for _, s := range r.States {
			counts[s]++
		}
	}
	if len(skills) == 0 {
		return "0 skills\n", "cali add <source>"
	}
	var parts []string
	for s, n := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", n, s))
	}
	sort.Strings(parts)
	switch {
	case counts[status.Missing] > 0 || counts[status.Unlocked] > 0:
		next = "cali sync"
	case counts[status.Outdated] > 0:
		next = "cali update --apply"
	case counts[status.Modified] > 0 || counts[status.BaseLost] > 0:
		next = "cali update"
	}
	return fmt.Sprintf("%d skills: %s\n", len(skills), strings.Join(parts, ", ")), next
}

func writeNext(w io.Writer, next string) {
	if next != "" {
		_, _ = fmt.Fprintf(w, "next: %s\n", next)
	}
}
