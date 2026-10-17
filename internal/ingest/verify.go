// Package ingest receives GitLab webhook deliveries over HTTP and applies the
// documented decision order (SYSARCH-001): body capture, signature
// verification, dedupe, parse, freshness, allowlist, routing, then a durable
// accept.
package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// signingTokenPrefix marks a Standard Webhooks signing token. GitLab shows
// and accepts its tokens in this form.
const signingTokenPrefix = "whsec_"

// The key sizes the Standard Webhooks specification allows. GitLab's keys
// are 32 bytes.
const (
	minSigningKeySize = 24
	maxSigningKeySize = 64
)

// ParseSigningToken decodes a signing token of the form whsec_<base64> into
// the HMAC key it carries. GitLab signs with the decoded key, not with the
// token's text. See "Verify webhook signatures" in GitLab's webhook
// documentation. The errors never repeat the token.
func ParseSigningToken(token string) ([]byte, error) {
	encoded, ok := strings.CutPrefix(token, signingTokenPrefix)
	if !ok {
		return nil, errors.New("must start with whsec_, exactly as GitLab shows the signing token")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("must be whsec_ followed by base64")
	}
	if len(key) < minSigningKeySize || len(key) > maxSigningKeySize {
		return nil, fmt.Errorf("must carry a key of %d to %d bytes, not %d", minSigningKeySize, maxSigningKeySize, len(key))
	}
	return key, nil
}

// Verify checks a Standard Webhooks signature (ADR-002): the HMAC-SHA256 over
// "{webhook-id}.{webhook-timestamp}.{body}" encoded as "v1,{base64}" entries
// in the webhook-signature header. The secrets are HMAC keys, as
// ParseSigningToken decodes them. Any one of them may sign (key rotation),
// and the header may carry several candidate signatures. The legacy
// X-Gitlab-Token is never consulted.
func Verify(secrets [][]byte, id, timestamp string, body []byte, header string) bool {
	if len(secrets) == 0 || id == "" || timestamp == "" || header == "" {
		return false
	}
	signed := strings.Join([]string{id, timestamp, string(body)}, ".")

	candidates := strings.Fields(header)
	for _, secret := range secrets {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(signed))
		want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
		for _, got := range candidates {
			if hmac.Equal([]byte(want), []byte(got)) {
				return true
			}
		}
	}
	return false
}
