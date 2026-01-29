package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewClient(apiKey, baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest) (string, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return "", errors.New("openrouter api key is missing")
	}
	if strings.TrimSpace(req.Model) == "" {
		return "", errors.New("openrouter model is required")
	}
	if len(req.Messages) == 0 {
		return "", errors.New("openrouter messages are required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	endpoint := c.baseURL + "/chat/completions"
	logRequest(endpoint, body)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		content, retry, err := c.doRequest(ctx, endpoint, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !retry {
			break
		}
		backoff := time.Duration(500*(attempt+1)) * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}

	return "", lastErr
}

func (c *Client) doRequest(ctx context.Context, endpoint string, payload []byte) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", false, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	if resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = resp.Status
		}
		err := fmt.Errorf("openrouter request failed: %s", message)
		return "", resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500, err
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", false, err
	}
	if len(decoded.Choices) == 0 {
		return "", false, errors.New("openrouter response missing choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", false, errors.New("openrouter response content is empty")
	}

	return content, false, nil
}
