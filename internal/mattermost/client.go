// Package mattermost is the bot-account HTTP client for posting and updating
// messages (SYSARCH-003). Only POST and PUT on /api/v4/posts are used; the
// error taxonomy here is the contract the delivery worker schedules against.
package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client posts as the bot account against {site-url}/api/v4.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client with a sane request timeout.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ThreadGoneError reports that the thread root post no longer exists (it was
// deleted intentionally per SYSREQ-026). Terminal: retrying cannot help.
type ThreadGoneError struct{}

func (*ThreadGoneError) Error() string { return "mattermost thread root is gone" }

// RateLimitedError reports a 429 carrying the server's reset instant; the
// worker must not retry before it (SYSREQ-018).
type RateLimitedError struct {
	Reset time.Time
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("mattermost rate limited, retry after %s", e.Reset.Format(time.RFC3339))
}

// APIError is any other non-success response. Retryable within the attempt
// budget (SYSREQ-016).
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mattermost post rejected: http %d: %s", e.Status, e.Body)
}

type postRequest struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	RootID    string `json:"root_id,omitempty"`
}

type postResponse struct {
	ID string `json:"id"`
}

// Post publishes one message, as a reply when rootID is set, and returns the
// created post id.
func (c *Client) Post(ctx context.Context, channelID, message, rootID string) (string, error) {
	body, err := json.Marshal(postRequest{ChannelID: channelID, Message: message, RootID: rootID})
	if err != nil {
		return "", fmt.Errorf("encode post for channel %q: %w", channelID, err)
	}
	return c.send(ctx, http.MethodPost, c.BaseURL+"/api/v4/posts", body)
}

// Update replaces the message of an already posted message (root status
// refresh, SYSREQ-004). The update is idempotent: reposting the same message
// text is a no-op a retry may safely repeat.
func (c *Client) Update(ctx context.Context, postID, message string) error {
	body, err := json.Marshal(struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}{ID: postID, Message: message})
	if err != nil {
		return fmt.Errorf("encode update for post %q: %w", postID, err)
	}
	_, err = c.send(ctx, http.MethodPut, c.BaseURL+"/api/v4/posts/"+postID, body)
	return err
}

// send executes a posts-API request and maps the response onto the worker's
// retry taxonomy (SYSREQ-016/018, SYSREQ-026).
func (c *Client) send(ctx context.Context, method, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", &TransportError{Err: err}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil {
		return "", &TransportError{Err: fmt.Errorf("read response body: %w", err)}
	}

	switch {
	case resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK:
		var pr postResponse
		if err := json.Unmarshal(respBody, &pr); err != nil || pr.ID == "" {
			return "", &APIError{Status: resp.StatusCode, Body: "response carried no post id"}
		}
		return pr.ID, nil

	case resp.StatusCode == http.StatusTooManyRequests:
		return "", &RateLimitedError{Reset: rateLimitReset(resp)}

	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone ||
		containsThreadGoneID(respBody):
		return "", &ThreadGoneError{}

	default:
		return "", &APIError{Status: resp.StatusCode, Body: truncate(string(respBody), 200)}
	}
}

// TransportError is a network-level failure (DNS, refused, timeout); always
// retryable (SYSREQ-016).
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "mattermost transport: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// rateLimitReset reads the RateLimit-Reset header (whole seconds until the
// caller may retry again). Absent or malformed resets fall back to "now",
// letting the backoff schedule take over.
func rateLimitReset(resp *http.Response) time.Time {
	secs, err := strconv.Atoi(resp.Header.Get("RateLimit-Reset"))
	if err != nil || secs < 0 {
		return time.Now()
	}
	return time.Now().Add(time.Duration(secs) * time.Second)
}

// containsThreadGoneID detects deletion-revealing error ids on statuses other
// than 404/410 (older servers answer 400 for missing roots).
func containsThreadGoneID(body []byte) bool {
	var e struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &e) != nil {
		return false
	}
	switch e.ID {
	case "api.post.get_post_for_thread.root_deleted.app_error",
		"model.post.post_not_found.app_default",
		"api.post.post_thread.root_deleted.app_error":
		return true
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
