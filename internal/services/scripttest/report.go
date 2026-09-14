package scripttest

import (
	"fmt"
	"io"
	"strings"
)

// minimumNameWidth keeps the ok/FAIL column in the same place for a suite whose
// case names are all short, so the report reads as a column rather than as
// ragged text.
const minimumNameWidth = 32

// Report writes one line per case, then a summary, and reports whether every
// case passed.
//
// The dotted leader is not decoration. A report is read by scanning the right
// hand column for FAIL, and a column that moves with the longest name in the
// file is one a reader has to scan twice.
func Report(out io.Writer, results []Result) bool {
	width := minimumNameWidth

	for _, result := range results {
		if len(result.Name) > width {
			width = len(result.Name)
		}
	}

	failed := 0

	for _, result := range results {
		verdict := "ok"

		if !result.Passed() {
			verdict = "FAIL"
			failed++
		}

		fmt.Fprintf(out, "  %s %s %s\n", result.Name, strings.Repeat(".", width+1-len(result.Name)), verdict)

		for _, failure := range result.Failures {
			fmt.Fprintf(out, "    - %s\n", failure)
		}
	}

	if failed == 0 {
		fmt.Fprintf(out, "\n%s, all ok\n", cases(len(results)))

		return true
	}

	fmt.Fprintf(out, "\n%s, %d FAILED\n", cases(len(results)), failed)

	return false
}

// cases pluralises the count so the summary line is a sentence rather than a
// template.
func cases(n int) string {
	if n == 1 {
		return "1 case"
	}

	return fmt.Sprintf("%d cases", n)
}
