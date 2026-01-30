package bitbucket

import "github.com/techitung-arunyawee/code-reviewer-2/internal/review"

type Config struct {
	Workspace   string
	RepoSlug    string
	PullRequest int
	Token       string
}

type PublishResult struct {
	CommentID string
	Error     error
}

type CommentPayload struct {
	Content Content `json:"content"`
}

type Content struct {
	Raw string `json:"raw"`
}

// Result is used to pass data to composer
type Result struct {
	Review review.Result
}
