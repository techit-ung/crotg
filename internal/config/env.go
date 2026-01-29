package config

import "os"

func OpenRouterAPIKey() string {
	return os.Getenv("OPENROUTER_API_KEY")
}

func OpenRouterBaseURL() string {
	return os.Getenv("OPENROUTER_BASE_URL")
}

func BitbucketToken() string {
	if token := os.Getenv("BITBUCKET_TOKEN"); token != "" {
		return token
	}

	return os.Getenv("BITBUCKET_ACCESS_TOKEN")
}
