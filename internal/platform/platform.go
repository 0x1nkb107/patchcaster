// Package platform names the forges and chat platforms PatchCaster connects
// to, and checks a connection's credential against its platform before an
// administrator stores it (SCEN-014). The platform adapters do the talking.
package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/patchcaster-app/patchcaster/internal/gitlab"
	"github.com/patchcaster-app/patchcaster/internal/mattermost"
)

// Kind names a platform.
type Kind string

// The platforms PatchCaster connects to so far.
const (
	GitLab     Kind = "gitlab"
	Mattermost Kind = "mattermost"
)

// IsForge reports whether the platform sends merge request events, as
// opposed to receiving messages.
func (k Kind) IsForge() bool {
	return k == GitLab
}

// NeedsCredential reports whether a connection to the platform must hold a
// credential. A chat connection posts with its bot's token. A forge
// connection may do without one, when its hooks are registered by hand.
func (k Kind) NeedsCredential() bool {
	return !k.IsForge()
}

// RefusedError reports a credential the platform refused, with the
// platform's answer.
type RefusedError struct {
	Platform Kind
	Answer   string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("%s refused the credential: %s", e.Platform, e.Answer)
}

// UnreachableError reports a platform that did not answer as one should at
// the base URL.
type UnreachableError struct {
	Platform Kind
	Err      error
}

func (e *UnreachableError) Error() string {
	return fmt.Sprintf("no %s answers at this address: %v", e.Platform, e.Err)
}

func (e *UnreachableError) Unwrap() error { return e.Err }

// ParseBaseURL reads a platform's base URL: an absolute http or https URL,
// which may have a path, as for a GitLab served below one. A trailing slash
// is dropped.
func ParseBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("must be an absolute http or https URL")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.New("must name no query, fragment or user")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

// Checker checks credentials against their platforms.
type Checker struct{}

// Check returns the account the credential acts as on the platform of kind
// at baseURL. It returns a *RefusedError when the platform refuses the
// credential, and an *UnreachableError when nothing answers like the
// platform.
func (Checker) Check(ctx context.Context, kind Kind, baseURL *url.URL, credential string) (string, error) {
	switch kind {
	case GitLab:
		account, err := gitlab.New(baseURL, credential).CurrentUser(ctx)
		return account, gitlabProblem(err)
	case Mattermost:
		account, err := mattermost.New(baseURL.String(), credential).Me(ctx)
		return account, mattermostProblem(err)
	}
	return "", fmt.Errorf("check connection: unknown platform %q", kind)
}

func gitlabProblem(err error) error {
	var refused *gitlab.APIError
	if errors.As(err, &refused) && isAuthStatus(refused.Status) {
		return &RefusedError{Platform: GitLab, Answer: refused.Message}
	}
	if err != nil {
		return &UnreachableError{Platform: GitLab, Err: err}
	}
	return nil
}

func mattermostProblem(err error) error {
	var refused *mattermost.APIError
	if errors.As(err, &refused) && isAuthStatus(refused.Status) {
		return &RefusedError{Platform: Mattermost, Answer: refused.Body}
	}
	if err != nil {
		return &UnreachableError{Platform: Mattermost, Err: err}
	}
	return nil
}

// isAuthStatus reports a status with which a platform refuses a credential.
func isAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}
