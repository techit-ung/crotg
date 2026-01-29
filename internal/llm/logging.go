package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/techitung-arunyawee/code-reviewer-2/internal/config"
)

type requestLogEntry struct {
	Timestamp string          `json:"timestamp"`
	Endpoint  string          `json:"endpoint"`
	Payload   json.RawMessage `json:"payload"`
}

func logRequest(endpoint string, payload []byte) {
	dir, err := config.CacheDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "llm-requests.log")
	entry := requestLogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Endpoint:  endpoint,
		Payload:   append([]byte(nil), payload...),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(data, '\n'))
}
