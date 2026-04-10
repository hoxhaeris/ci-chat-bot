package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/openshift/ci-chat-bot/pkg/slack/parser"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	// threadTTL is how long we track AI threads before they expire.
	threadTTL = 24 * time.Hour
	// threadCleanupInterval is how often we run the cleanup goroutine.
	threadCleanupInterval = 1 * time.Hour
)

// AIClient is an HTTP client for the AI assistant service.
type AIClient struct {
	serviceURL string
	httpClient *http.Client

	// aiThreads tracks thread timestamps that were started by AI responses,
	// so we can identify follow-up messages in those threads.
	// The value is the time the thread was first tracked.
	aiThreads   map[string]time.Time
	aiThreadsMu sync.RWMutex
}

// AskRequest is the request body for the AI service /ask endpoint.
type AskRequest struct {
	Question  string `json:"question"`
	UserID    string `json:"user_id"`
	ThreadID  string `json:"thread_id"`
	Context   string `json:"context,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}

// AskResponse is the response body from the AI service /ask endpoint.
type AskResponse struct {
	Answer    string `json:"answer"`
	RequestID string `json:"request_id,omitempty"`
}

// NewAIClient creates a new AI assistant HTTP client.
func NewAIClient(serviceURL string) *AIClient {
	c := &AIClient{
		serviceURL: serviceURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		aiThreads: make(map[string]time.Time),
	}
	go c.cleanupOldThreads()
	return c
}

// cleanupOldThreads periodically removes thread entries older than threadTTL.
func (c *AIClient) cleanupOldThreads() {
	ticker := time.NewTicker(threadCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		c.aiThreadsMu.Lock()
		cutoff := time.Now().Add(-threadTTL)
		removed := 0
		for ts, created := range c.aiThreads {
			if created.Before(cutoff) {
				delete(c.aiThreads, ts)
				removed++
			}
		}
		c.aiThreadsMu.Unlock()
		if removed > 0 {
			klog.V(4).Infof("Cleaned up %d expired AI thread entries", removed)
		}
	}
}

// retryBackoff is the exponential backoff configuration for AI service requests.
var retryBackoff = wait.Backoff{
	Steps:    3,
	Duration: 1 * time.Second,
	Factor:   2.0,
	Jitter:   0.1,
}

// isRetryableError returns true if the HTTP status code indicates a transient error
// worth retrying (5xx server errors). Client errors (4xx) are not retried.
func isRetryableStatusCode(statusCode int) bool {
	return statusCode >= 500
}

// Ask sends a question to the AI assistant and returns the answer.
// Retries on transient failures (connection errors, 5xx) with exponential backoff.
func (c *AIClient) Ask(req AskRequest) (*AskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	var askResp AskResponse

	err = wait.ExponentialBackoff(retryBackoff, func() (bool, error) {
		url := fmt.Sprintf("%s/ask", c.serviceURL)
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return false, fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			// Connection error — retryable
			lastErr = fmt.Errorf("failed to call AI service: %w", err)
			klog.V(2).Infof("AI service request failed (will retry): %v", lastErr)
			return false, nil
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			return false, nil
		}

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusTooManyRequests {
				// Rate limited — do not retry
				return false, fmt.Errorf("AI service is rate limited, please try again later")
			}
			lastErr = fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(respBody))
			if isRetryableStatusCode(resp.StatusCode) {
				klog.V(2).Infof("AI service returned %d (will retry): %s", resp.StatusCode, string(respBody))
				return false, nil
			}
			// 4xx — do not retry
			return false, lastErr
		}

		if err := json.Unmarshal(respBody, &askResp); err != nil {
			return false, fmt.Errorf("failed to parse AI response: %w", err)
		}

		return true, nil
	})

	if err != nil {
		if err == wait.ErrWaitTimeout && lastErr != nil {
			return nil, fmt.Errorf("AI service request failed after retries: %w", lastErr)
		}
		return nil, err
	}

	return &askResp, nil
}

// HealthCheck checks if the AI service is healthy.
func (c *AIClient) HealthCheck() error {
	url := fmt.Sprintf("%s/health/live", c.serviceURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("AI service health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AI service unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

// TrackAIThread marks a thread timestamp as an AI conversation thread.
func (c *AIClient) TrackAIThread(threadTS string) {
	c.aiThreadsMu.Lock()
	defer c.aiThreadsMu.Unlock()
	c.aiThreads[threadTS] = time.Now()
	klog.V(4).Infof("Tracking AI thread: %s", threadTS)
}

// IsAIThread checks if a thread timestamp belongs to an AI conversation.
func (c *AIClient) IsAIThread(threadTS string) bool {
	c.aiThreadsMu.RLock()
	defer c.aiThreadsMu.RUnlock()
	_, ok := c.aiThreads[threadTS]
	return ok
}

// IsConfigured returns true if the AI service URL is set.
func (c *AIClient) IsConfigured() bool {
	return c.serviceURL != ""
}

// HandleThreadFollowUp handles a follow-up message in an existing AI thread.
func (c *AIClient) HandleThreadFollowUp(client parser.SlackClient, event *slackevents.MessageEvent) {
	HandleAIThreadFollowUp(client, c, event)
}
