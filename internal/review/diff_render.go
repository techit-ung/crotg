package review

import (
	"strings"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/git"
)

func RenderUnifiedDiffFile(file git.DiffFile) string {
	var builder strings.Builder
	builder.WriteString("diff --git a/")
	builder.WriteString(file.Path)
	builder.WriteString(" b/")
	builder.WriteString(file.Path)
	builder.WriteString("\n--- a/")
	builder.WriteString(file.Path)
	builder.WriteString("\n+++ b/")
	builder.WriteString(file.Path)
	builder.WriteString("\n")

	for _, hunk := range file.Hunks {
		builder.WriteString(hunk.Header)
		builder.WriteString("\n")
		for _, line := range hunk.Lines {
			switch line.Kind {
			case git.DiffLineAdd:
				builder.WriteString("+")
			case git.DiffLineDel:
				builder.WriteString("-")
			default:
				builder.WriteString(" ")
			}
			builder.WriteString(line.Text)
			builder.WriteString("\n")
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}
