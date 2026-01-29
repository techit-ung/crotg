package review

import (
	"fmt"
	"strings"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/llm"
)

const fileReviewSchema = `{
  "comments": [
    {
      "filePath": "path",
      "startLine": 10,
      "endLine": 10,
      "severity": "BLOCKER",
      "title": "Short title",
      "body": "Detailed comment",
      "suggestion": "Optional suggestion",
      "evidence": "Optional snippet",
      "tags": ["optional", "tags"]
    }
  ]
}`

const verdictSchema = `{
  "verdict": {
    "decision": "GO",
    "summary": "Short summary",
    "rationale": ["..."]
  }
}`

func BuildFileReviewMessages(guidelines, diff string) []llm.Message {
	system := strings.Join([]string{
		"You are a expert senior software engineer. You are tasked to review the code",
		"Follow the provided guidelines.",
		"Return JSON only. Do not include markdown fences.",
	}, " ")

	user := fmt.Sprintf(strings.Join([]string{
		"Guidelines:",
		"%s",
		"",
		"Severity scale: NIT (minor), SUGGESTION (improvement), ISSUE (bug/maintainability), BLOCKER (must-fix).",
		"Review the diff and return comments in the schema below.",
		"If there are no comments, return {\"comments\": []}.",
		"Schema:",
		"%s",
		"",
		"Diff:",
		"%s",
	}, "\n"), guidelines, fileReviewSchema, diff)

	return []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

func BuildVerdictMessages(guidelines string, comments []Comment, stats Stats, ruleDecision Decision) []llm.Message {
	system := strings.Join([]string{
		"You are a expert senior software engineer. You are tasked to review the code",
		"Return JSON only. Do not include markdown fences.",
	}, " ")

	lines := make([]string, 0, len(comments))
	for _, comment := range comments {
		lines = append(lines, fmt.Sprintf("- [%s] %s:%d %s", comment.Severity, comment.FilePath, comment.StartLine, comment.Title))
	}
	if len(lines) == 0 {
		lines = append(lines, "- No comments.")
	}

	user := fmt.Sprintf(strings.Join([]string{
		"Guidelines:",
		"%s",
		"",
		"Comment summary:",
		"%s",
		"",
		"Stats: NIT=%d, SUGGESTION=%d, ISSUE=%d, BLOCKER=%d.",
		"Rule-based decision is %s if any BLOCKER exists, otherwise GO.",
		"Provide a verdict JSON matching this schema:",
		"%s",
	}, "\n"), guidelines, strings.Join(lines, "\n"), stats.Nit, stats.Suggestion, stats.Issue, stats.Blocker, ruleDecision, verdictSchema)

	return []llm.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}
