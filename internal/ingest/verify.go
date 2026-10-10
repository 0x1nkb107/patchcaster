// Package ingest receives GitLab webhook deliveries over HTTP and applies the
// documented decision order (SYSARCH-001): body capture, signature
// verification, dedupe, parse, freshness, allowlist, routing, then a durable
// accept.
package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// Verify checks a Standard Webhooks signature (ADR-002): the HMAC-SHA256 over
// "{webhook-id}.{webhook-timestamp}.{body}" encoded as "v1,{base64}" entries
// in the webhook-signature header. Any one configured secret may sign (key
// rotation), and the header may carry several candidate signatures. The legacy
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
