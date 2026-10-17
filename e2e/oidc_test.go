package e2e

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The client PatchCaster is registered as at the fake provider.
const (
	oidcClientID     = "patchcaster"
	oidcClientSecret = "e2e-client-secret-that-must-stay-sealed"
)

// identity is who the fake provider says a person is.
type identity struct {
	subject string
	name    string
	groups  []string
}

// tamper names one way an ID token can fail PatchCaster's checks.
type tamper int

const (
	honest tamper = iota
	foreignSignature
	foreignAudience
	foreignIssuer
	expired
)

// grant is an authorization code the fake provider issued and what it
// stands for.
type grant struct {
	who         identity
	challenge   string
	redirectURI string
	nonce       string
}

// fakeProvider is an OpenID Connect provider with the authorization code
// flow and PKCE, as PatchCaster uses it (ADR-033). It authenticates whoever
// the test names next, without a login page, and signs ID tokens with RS256.
type fakeProvider struct {
	server     *httptest.Server
	key        *rsa.PrivateKey
	foreignKey *rsa.PrivateKey

	mu     sync.Mutex
	next   identity
	tamper tamper
	grants map[string]grant
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	p := &fakeProvider{
		key:        newRSAKey(t),
		foreignKey: newRSAKey(t),
		grants:     map[string]grant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.keys)
	mux.HandleFunc("GET /authorize", p.authorize)
	mux.HandleFunc("POST /token", p.token)
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// issuer is the provider's issuer URL.
func (p *fakeProvider) issuer() string {
	return p.server.URL
}

// authenticateAs makes the next login authenticate who, with ID tokens
// spoilt as tamper says.
func (p *fakeProvider) authenticateAs(who identity, how tamper) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next = who
	p.tamper = how
}

func (p *fakeProvider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.issuer(),
		"authorization_endpoint":                p.issuer() + "/authorize",
		"token_endpoint":                        p.issuer() + "/token",
		"jwks_uri":                              p.issuer() + "/keys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
	})
}

func (p *fakeProvider) keys(w http.ResponseWriter, _ *http.Request) {
	public := p.key.PublicKey
	writeJSON(w, http.StatusOK, map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": "e2e",
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(public.E)).Bytes()),
		}},
	})
}

// authorize authenticates the next identity at once and sends the browser
// back with a code. It insists on PKCE, so a login without it fails.
func (p *fakeProvider) authorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	valid := query.Get("response_type") == "code" &&
		query.Get("client_id") == oidcClientID &&
		query.Get("code_challenge_method") == "S256" &&
		query.Get("code_challenge") != ""
	if !valid {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	code := rand.Text()
	p.mu.Lock()
	p.grants[code] = grant{
		who:         p.next,
		challenge:   query.Get("code_challenge"),
		redirectURI: query.Get("redirect_uri"),
		nonce:       query.Get("nonce"),
	}
	p.mu.Unlock()

	back, err := url.Parse(query.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	backQuery := back.Query()
	backQuery.Set("code", code)
	backQuery.Set("state", query.Get("state"))
	back.RawQuery = backQuery.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// token exchanges a code for an ID token, once, for the registered client
// and the PKCE verifier that matches the code's challenge.
func (p *fakeProvider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if !clientAuthenticated(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	p.mu.Lock()
	g, ok := p.grants[r.PostForm.Get("code")]
	delete(p.grants, r.PostForm.Get("code"))
	how := p.tamper
	p.mu.Unlock()

	verified := ok &&
		g.redirectURI == r.PostForm.Get("redirect_uri") &&
		pkceChallenge(r.PostForm.Get("code_verifier")) == g.challenge
	if !verified {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	idToken, err := p.idToken(g, how)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": rand.Text(),
		"token_type":   "Bearer",
		"expires_in":   300,
		"id_token":     idToken,
	})
}

// clientAuthenticated accepts the registered client's credentials, sent
// either way the discovery document offers.
func clientAuthenticated(r *http.Request) bool {
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	return id == oidcClientID && secret == oidcClientSecret
}

// pkceChallenge is the S256 challenge of a verifier (RFC 7636).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// idToken signs the ID token of a grant, spoilt as how says.
func (p *fakeProvider) idToken(g grant, how tamper) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"iss":    p.issuer(),
		"sub":    g.who.subject,
		"aud":    oidcClientID,
		"iat":    now.Unix(),
		"exp":    now.Add(5 * time.Minute).Unix(),
		"name":   g.who.name,
		"email":  g.who.subject + "@example.com",
		"groups": g.who.groups,
	}
	if g.nonce != "" {
		claims["nonce"] = g.nonce
	}
	signer := p.key
	switch how {
	case foreignSignature:
		signer = p.foreignKey
	case foreignAudience:
		claims["aud"] = "another-client"
	case foreignIssuer:
		claims["iss"] = "https://another-provider.example"
	case expired:
		claims["iat"] = now.Add(-time.Hour).Unix()
		claims["exp"] = now.Add(-30 * time.Minute).Unix()
	}
	return signJWT(signer, claims)
}

// signJWT encodes claims as a compact JWS signed with RS256 (RFC 7515).
func signJWT(key *rsa.PrivateKey, claims map[string]any) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "e2e", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
