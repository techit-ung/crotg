package bitbucket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) PublishComment(markdown string) (string, error) {
	url := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%d/comments",
		c.config.Workspace, c.config.RepoSlug, c.config.PullRequest)

	payload := CommentPayload{
		Content: Content{
			Raw: markdown,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.config.Token))

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %s: %s", resp.Status, string(body))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Bitbucket might return a string ID or something else, but we just want to know it succeeded
		return "success", nil
	}

	return fmt.Sprintf("%d", result.ID), nil
}
