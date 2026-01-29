package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Severity string

const (
	SeverityNit        Severity = "NIT"
	SeveritySuggestion Severity = "SUGGESTION"
	SeverityIssue      Severity = "ISSUE"
	SeverityBlocker    Severity = "BLOCKER"
)

type Decision string

const (
	DecisionGo   Decision = "GO"
	DecisionNoGo Decision = "NO_GO"
)

type Comment struct {
	ID         string
	FilePath   string
	StartLine  int
	EndLine    int
	Severity   Severity
	Title      string
	Body       string
	Suggestion *string
	Evidence   *string
	Tags       []string
	Publish    bool
}

type Verdict struct {
	Decision  Decision
	Summary   string
	Rationale []string
	Stats     Stats
}

type Stats struct {
	Nit        int
	Suggestion int
	Issue      int
	Blocker    int
}

type Result struct {
	Comments      []Comment
	Verdict       Verdict
	Model         string
	GuidelineHash string
	Dropped       int
	GeneratedAt   time.Time
}

func ComputeStats(comments []Comment) Stats {
	stats := Stats{}
	for _, comment := range comments {
		switch comment.Severity {
		case SeverityNit:
			stats.Nit++
		case SeveritySuggestion:
			stats.Suggestion++
		case SeverityIssue:
			stats.Issue++
		case SeverityBlocker:
			stats.Blocker++
		}
	}
	return stats
}

func StableCommentID(comment Comment) string {
	parts := []string{
		strings.TrimSpace(comment.FilePath),
		fmt.Sprintf("%d", comment.StartLine),
		fmt.Sprintf("%d", comment.EndLine),
		string(comment.Severity),
		strings.TrimSpace(comment.Title),
		strings.TrimSpace(comment.Body),
	}
	hasher := sha256.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func NormalizeDecision(value string) Decision {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "NO_GO", "NO-GO", "NOGO":
		return DecisionNoGo
	default:
		return DecisionGo
	}
}

func NormalizeSeverity(value string) Severity {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "BLOCKER":
		return SeverityBlocker
	case "ISSUE":
		return SeverityIssue
	case "SUGGESTION":
		return SeveritySuggestion
	default:
		return SeverityNit
	}
}
