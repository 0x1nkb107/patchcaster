package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"testing"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// sessionCookie names PatchCaster's session cookie.
const sessionCookie = "patchcaster_session"

// browser is one person's browser against one service. It keeps cookies
// and follows redirects, as the login needs.
type browser struct {
	svc    *service
	jar    *cookiejar.Jar
	client *http.Client
}

func newBrowser(t *testing.T, svc *service) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &browser{
		svc:    svc,
		jar:    jar,
		client: &http.Client{Jar: jar},
	}
}

// session returns the browser's session cookie, or nil without one.
func (b *browser) session(t *testing.T) *http.Cookie {
	t.Helper()
	interfaceURL, err := url.Parse(b.svc.managementURL)
	require.NoError(t, err)
	for _, cookie := range b.jar.Cookies(interfaceURL) {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	return nil
}

// sessionSource hands the browser's session cookie to the generated client.
type sessionSource struct {
	t *testing.T
	b *browser
}

// Session returns the session cookie's value. The generated client sets the
// cookie itself, and the jar adds the same cookie once more, which servers
// read as the same value.
func (s sessionSource) Session(context.Context, apiv1.OperationName) (apiv1.Session, error) {
	cookie := s.b.session(s.t)
	if cookie == nil {
		return apiv1.Session{}, ogenerrors.ErrSkipClientSecurity
	}
	return apiv1.Session{APIKey: cookie.Value}, nil
}

// api returns the management API client of this browser, generated from
// the same description as the server (ADR-031). It shares the browser's
// cookies, so the setup and the login that follows share one session.
func (b *browser) api(t *testing.T) *apiv1.Client {
	t.Helper()
	client, err := apiv1.NewClient(b.svc.managementURL, sessionSource{t: t, b: b}, apiv1.WithClient(b.client))
	require.NoError(t, err)
	return client
}

// page is where a browser ended up after following redirects.
type page struct {
	status int
	path   string
	body   string
}

// visit follows url and its redirects as a browser does.
func (b *browser) visit(t *testing.T, url string) page {
	t.Helper()
	resp, err := b.client.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return page{
		status: resp.StatusCode,
		path:   resp.Request.URL.Path,
		body:   string(body),
	}
}

// logIn runs a login through the provider, as whoever it authenticates.
func (b *browser) logIn(t *testing.T) page {
	t.Helper()
	return b.visit(t, b.svc.managementURL+"/auth/login")
}

// requireLoggedIn asserts that a login ended at the interface's root with a
// session.
func (b *browser) requireLoggedIn(t *testing.T, landed page) {
	t.Helper()
	require.Equal(t, "/", landed.path, "a login lands at the interface root: %d %s", landed.status, landed.body)
	require.NotNil(t, b.session(t))
}

// setupLink matches the link the bootstrap command prints and captures its
// token.
var setupLink = regexp.MustCompile(`https?://\S+/setup#(\S+)`)

// bootstrap runs the bootstrap command for svc and returns the setup token
// from the link it prints.
func bootstrap(t *testing.T, svc *service) string {
	t.Helper()
	out, code := runPatchcaster(t, "bootstrap", "--database-url", databaseURL, "--interface-url", svc.managementURL)
	require.Zero(t, code, out)
	match := setupLink.FindStringSubmatch(out)
	require.NotNil(t, match, "bootstrap prints a setup link: %s", out)
	return match[1]
}

// setupRequest names the fake provider for the setup with token.
func setupRequest(token string, provider *fakeProvider) *apiv1.SetupRequest {
	return &apiv1.SetupRequest{
		SetupToken:   token,
		Issuer:       provider.issuer(),
		ClientID:     oidcClientID,
		ClientSecret: oidcClientSecret,
	}
}

// setUp runs the whole setup with req and makes who the first
// administrator. It returns who's browser.
func setUp(t *testing.T, svc *service, provider *fakeProvider, req *apiv1.SetupRequest, who identity) *browser {
	t.Helper()
	admin := newBrowser(t, svc)
	started, err := admin.api(t).SetUp(context.Background(), req)
	require.NoError(t, err)
	provider.authenticateAs(who, honest)
	admin.requireLoggedIn(t, admin.visit(t, started.LoginURL))
	return admin
}
