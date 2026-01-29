package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/git"
	"github.com/techitung-arunyawee/code-reviewer-2/internal/llm"
)

const DefaultModel = "openai/gpt-4o-mini"

type Progress struct {
	Completed   int
	Total       int
	CurrentFile string
}

type RunOptions struct {
	Model          string
	GuidelinePaths []string
	FreeText       string
	GuidelineHash  string
	MaxConcurrency int
}

type fileReviewResult struct {
	comments []Comment
	err      error
	filePath string
}

func Run(ctx context.Context, client *llm.Client, files []git.DiffFile, opts RunOptions, progress func(Progress)) (Result, error) {
	if len(files) == 0 {
		return Result{}, errors.New("no diff files to review")
	}
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 3
	}
	if opts.GuidelineHash == "" {
		hash, err := HashGuidelines(opts.GuidelinePaths, opts.FreeText)
		if err != nil {
			return Result{}, err
		}
		opts.GuidelineHash = hash
	}

	guidelines, err := LoadGuidelines(opts.GuidelinePaths, opts.FreeText)
	if err != nil {
		return Result{}, err
	}

	jobs := make(chan git.DiffFile)
	results := make(chan fileReviewResult)

	worker := func() {
		for file := range jobs {
			if len(file.Hunks) == 0 {
				results <- fileReviewResult{comments: nil, filePath: file.Path}
				continue
			}
			diff := RenderUnifiedDiffFile(file)
			messages := BuildFileReviewMessages(guidelines, diff)
			content, err := client.ChatCompletion(ctx, llm.ChatRequest{
				Model:       opts.Model,
				Messages:    messages,
				Temperature: 0.2,
			})
			if err != nil {
				results <- fileReviewResult{err: err, filePath: file.Path}
				continue
			}

			comments, err := parseFileComments(content)
			results <- fileReviewResult{comments: comments, err: err, filePath: file.Path}
		}
	}

	for i := 0; i < opts.MaxConcurrency; i++ {
		go worker()
	}

	go func() {
		for _, file := range files {
			jobs <- file
		}
		close(jobs)
	}()

	collected := make([]Comment, 0)
	var firstErr error

	total := len(files)
	completed := 0
	for completed < total {
		result := <-results
		completed++
		if progress != nil {
			progress(Progress{Completed: completed, Total: total, CurrentFile: result.filePath})
		}
		if result.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("review failed for %s: %w", result.filePath, result.err)
		}
		collected = append(collected, result.comments...)
	}

	if firstErr != nil {
		return Result{}, firstErr
	}

	deduped := dedupeComments(collected)
	stats := ComputeStats(deduped)
	ruleDecision := DecisionGo
	if stats.Blocker > 0 {
		ruleDecision = DecisionNoGo
	}

	verdict, err := generateVerdict(ctx, client, opts.Model, guidelines, deduped, stats, ruleDecision)
	if err != nil {
		verdict = Verdict{
			Decision:  ruleDecision,
			Summary:   "Verdict unavailable due to parsing error.",
			Rationale: []string{"Defaulted to rule-based decision."},
			Stats:     stats,
		}
	} else {
		finalDecision := verdict.Decision
		if ruleDecision == DecisionNoGo || verdict.Decision == DecisionNoGo {
			finalDecision = DecisionNoGo
		}
		verdict.Decision = finalDecision
		verdict.Stats = stats
		if strings.TrimSpace(verdict.Summary) == "" {
			verdict.Summary = "Summary unavailable."
		}
	}

	return Result{
		Comments:      deduped,
		Verdict:       verdict,
		Model:         opts.Model,
		GuidelineHash: opts.GuidelineHash,
		GeneratedAt:   time.Now(),
	}, nil
}

func dedupeComments(comments []Comment) []Comment {
	seen := make(map[string]Comment)
	for _, comment := range comments {
		if strings.TrimSpace(comment.ID) == "" {
			comment.ID = StableCommentID(comment)
		}
		if _, ok := seen[comment.ID]; ok {
			continue
		}
		seen[comment.ID] = comment
	}

	deduped := make([]Comment, 0, len(seen))
	for _, comment := range seen {
		deduped = append(deduped, comment)
	}
	return deduped
}

func parseFileComments(content string) ([]Comment, error) {
	payload := stripCodeFence(content)
	var decoded struct {
		Comments []struct {
			FilePath   string   `json:"filePath"`
			StartLine  int      `json:"startLine"`
			EndLine    int      `json:"endLine"`
			Severity   string   `json:"severity"`
			Title      string   `json:"title"`
			Body       string   `json:"body"`
			Suggestion *string  `json:"suggestion"`
			Evidence   *string  `json:"evidence"`
			Tags       []string `json:"tags"`
		} `json:"comments"`
	}

	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return nil, err
	}

	comments := make([]Comment, 0, len(decoded.Comments))
	for _, item := range decoded.Comments {
		comment := Comment{
			FilePath:   strings.TrimSpace(item.FilePath),
			StartLine:  item.StartLine,
			EndLine:    item.EndLine,
			Severity:   NormalizeSeverity(item.Severity),
			Title:      strings.TrimSpace(item.Title),
			Body:       strings.TrimSpace(item.Body),
			Suggestion: trimOptional(item.Suggestion),
			Evidence:   trimOptional(item.Evidence),
			Tags:       item.Tags,
			Publish:    true,
		}
		if comment.StartLine <= 0 || comment.EndLine <= 0 || comment.EndLine < comment.StartLine {
			continue
		}
		if comment.FilePath == "" || comment.Title == "" || comment.Body == "" {
			continue
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

func generateVerdict(ctx context.Context, client *llm.Client, model, guidelines string, comments []Comment, stats Stats, ruleDecision Decision) (Verdict, error) {
	content, err := client.ChatCompletion(ctx, llm.ChatRequest{
		Model:       model,
		Messages:    BuildVerdictMessages(guidelines, comments, stats, ruleDecision),
		Temperature: 0.2,
	})
	if err != nil {
		return Verdict{}, err
	}

	payload := stripCodeFence(content)
	var decoded struct {
		Verdict struct {
			Decision  string   `json:"decision"`
			Summary   string   `json:"summary"`
			Rationale []string `json:"rationale"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return Verdict{}, err
	}

	return Verdict{
		Decision:  NormalizeDecision(decoded.Verdict.Decision),
		Summary:   strings.TrimSpace(decoded.Verdict.Summary),
		Rationale: decoded.Verdict.Rationale,
		Stats:     stats,
	}, nil
}

func stripCodeFence(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
		if idx := strings.Index(trimmed, "\n"); idx != -1 {
			trimmed = trimmed[idx+1:]
		}
		if end := strings.LastIndex(trimmed, "```"); end != -1 {
			trimmed = trimmed[:end]
		}
	}
	return strings.TrimSpace(trimmed)
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
