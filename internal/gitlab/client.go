// Package gitlab talks to the GitLab REST API on behalf of a forge
// connection (SYSARCH-007). It checks the connection's token and registers
// the hooks that deliver the connection's events (SYSREQ-044).
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// endpoint returns the address of a resource below /api/v4. Each segment
// is escaped on its own, so that a path such as "group/project" can name a
// project, as GitLab expects it.
func (c *Client) endpoint(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	u := *c.baseURL
	u.RawPath = strings.TrimSuffix(c.baseURL.EscapedPath(), "/") + "/api/v4/" + strings.Join(escaped, "/")
	u.Path = strings.TrimSuffix(c.baseURL.Path, "/") + "/api/v4/" + strings.Join(segments, "/")
	return u.String()
}

// answer is what GitLab sent back: the status, the headers and the body.
type answer struct {
	status int
	header http.Header
	body   []byte
}

// do sends one request with an optional JSON body and returns GitLab's
// answer. Only a missing answer is an error here.
func (c *Client) do(ctx context.Context, method, endpoint string, payload any) (*answer, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode gitlab request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build gitlab request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	read, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &TransportError{Err: fmt.Errorf("read answer: %w", err)}
	}
	return &answer{status: resp.StatusCode, header: resp.Header, body: read}, nil
}

// expect fails unless GitLab answered with status, and then decodes the
// body into out, when out is not nil.
func (a *answer) expect(status int, out any) error {
	if a.status != status {
		return &APIError{Status: a.status, Message: messageOf(a.body)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(a.body, out); err != nil {
		return &APIError{Status: a.status, Message: "the answer is not the JSON GitLab sends"}
	}
	return nil
}

// get reads one resource below /api/v4 into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	a, err := c.do(ctx, http.MethodGet, c.endpoint(path), nil)
	if err != nil {
		return err
	}
	return a.expect(http.StatusOK, out)
}

// nextPage returns the page after the one a list answer holds, or 0 on the
// last page.
func (a *answer) nextPage() int {
	next, err := strconv.Atoi(a.header.Get("X-Next-Page"))
	if err != nil {
		return 0
	}
	return next
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
