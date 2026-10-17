// Package gitlab talks to the GitLab REST API on behalf of a forge
// connection (SYSARCH-007). It checks the connection's token now, and will
// register hooks later.
package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client calls {base URL}/api/v4 with a token, which may be a personal,
// group or project access token.
type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

// New returns a client for the GitLab at baseURL, with a request timeout.
func New(baseURL *url.URL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// APIError is an answer other than success, with the message GitLab gave.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gitlab answered http %d: %s", e.Status, e.Message)
}

// TransportError is a request that got no answer at all.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "reach gitlab: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// CurrentUser returns the username the token acts as.
func (c *Client) CurrentUser(ctx context.Context) (string, error) {
	var user struct {
		Username string `json:"username"`
	}
	if err := c.get(ctx, "user", &user); err != nil {
		return "", err
	}
	if user.Username == "" {
		return "", &APIError{Status: http.StatusOK, Message: "the answer named no user"}
	}
	return user.Username, nil
}

// get reads one resource below /api/v4 into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	endpoint := c.baseURL.JoinPath("api", "v4", path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build gitlab request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return &TransportError{Err: fmt.Errorf("read answer: %w", err)}
	}
	if resp.StatusCode != http.StatusOK {
		return &APIError{Status: resp.StatusCode, Message: messageOf(body)}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &APIError{Status: resp.StatusCode, Message: "the answer is not the JSON GitLab sends"}
	}
	return nil
}

// messageOf reads the message of a GitLab error answer, which carries it in
// "message" or "error", and falls back to the start of the body.
func messageOf(body []byte) string {
	var answer struct {
		Message any    `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &answer) == nil {
		if text, ok := answer.Message.(string); ok && text != "" {
			return text
		}
		if answer.Error != "" {
			return answer.Error
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}
