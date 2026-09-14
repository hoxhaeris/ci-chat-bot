package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/openshift/ci-chat-bot/pkg/slack/parser"
	"github.com/slack-go/slack/slackevents"
	"k8s.io/klog"
)

const (
	// threadTTL is how long we track AI threads before they expire.
	threadTTL = 24 * time.Hour
	// threadCleanupInterval is how often we run the cleanup goroutine.
	threadCleanupInterval = 1 * time.Hour
	// maxConcurrentAIRequests limits the number of simultaneous AI service calls
	// to prevent resource exhaustion if users send many messages while the service is slow.
	maxConcurrentAIRequests = 10
	// askTimeout bounds a single /ask call. The research path (ship-help-bot
	// persona + oversight) is ~150s typical but can exceed 200s with oversight
	// retries; this is the outer bound, above the researcher's own
	// SHIP_HELP_MCP_TIMEOUT default (~300s).
	askTimeout = 360 * time.Second
	// healthCheckTimeout bounds the liveness probe.
	healthCheckTimeout = 10 * time.Second
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

	// sem limits the number of concurrent AI requests.
	sem chan struct{}
}

// AskRequest is the request body for the AI service /ask endpoint.
type AskRequest struct {
	Question  string `json:"question"`
	UserID    string `json:"user_id"`
	ThreadID  string `json:"thread_id"`
	Context   string `json:"context,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}

// AIButton is a follow-up quick-action button proposed by the AI assistant.
// The Go bot renders it as a Slack button; clicking it re-asks with Message.
type AIButton struct {
	Label   string `json:"label"`
	Message string `json:"message"`
	Style   string `json:"style,omitempty"`
}

// AICommand is a validated cluster-bot command the AI assistant offers as a
// one-click "▶ Run" button; clicking it executes Command as if the user typed it.
type AICommand struct {
	Label   string `json:"label,omitempty"`
	Command string `json:"command"`
}

// AskResponse is the response body from the AI service /ask endpoint.
type AskResponse struct {
	Answer    string      `json:"answer"`
	Buttons   []AIButton  `json:"buttons,omitempty"`
	Commands  []AICommand `json:"commands,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

// NewAIClient creates a new AI assistant HTTP client.
func NewAIClient(serviceURL string) *AIClient {
	c := &AIClient{
		serviceURL: serviceURL,
		// No client-level timeout: per-request context deadlines bound each call
		// (askTimeout / healthCheckTimeout). A fixed 120s client timeout would cut
		// off the research path, which can legitimately run longer.
		httpClient: &http.Client{},
		aiThreads:  make(map[string]time.Time),
		sem:        make(chan struct{}, maxConcurrentAIRequests),
	}
	go c.cleanupOldThreads()
	return c
}

// acquireSem blocks until a concurrency slot is available.
// Returns false if the client is nil or not configured.
func (c *AIClient) acquireSem() bool {
	if c == nil || !c.IsConfigured() {
		return false
	}
	c.sem <- struct{}{}
	return true
}

// releaseSem frees a concurrency slot.
func (c *AIClient) releaseSem() {
	<-c.sem
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

// Ask sends a question to the AI assistant and returns the answer.
//
// Only a single attempt is made: the research path is expensive (it can run
// well over a minute), so retrying would risk duplicate heavy work. The call is
// bounded by askTimeout via a per-request context.
func (c *AIClient) Ask(req AskRequest) (*AskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/ask", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("AI service is rate limited, please try again later")
		}
		return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var askResp AskResponse
	if err := json.Unmarshal(respBody, &askResp); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}
	return &askResp, nil
}

// HealthCheck checks if the AI service is healthy.
func (c *AIClient) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()
	url := fmt.Sprintf("%s/health/live", c.serviceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
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
// Must be called from a goroutine — blocks until a concurrency slot is available.
func (c *AIClient) HandleThreadFollowUp(client parser.SlackClient, event *slackevents.MessageEvent) {
	if !c.acquireSem() {
		return
	}
	defer c.releaseSem()
	HandleAIThreadFollowUp(client, c, event)
}

// HandleErrorSuggestion posts an AI-generated suggestion as a threaded reply
// to a command error message. Must be called from a goroutine.
func (c *AIClient) HandleErrorSuggestion(client parser.SlackClient, channel, parentTS, userCommand, errorMessage string) {
	if !c.acquireSem() {
		return
	}
	defer c.releaseSem()
	handleErrorSuggestion(c, client, channel, parentTS, userCommand, errorMessage)
}
