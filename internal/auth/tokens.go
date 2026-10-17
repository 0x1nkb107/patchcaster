package auth

import "crypto/rand"

// TokenPrefix starts every API token, so that people and secret scanners
// recognize a leaked one.
const TokenPrefix = "patchcaster_"

// NewToken returns a new API token and the hash under which it is stored.
// Only the owner ever sees the token, once (SYSREQ-040).
func NewToken() (token string, hash []byte) {
	token = TokenPrefix + rand.Text()
	return token, TokenHash(token)
}

// TokenHash returns the hash under which an API token is stored and looked
// up.
func TokenHash(token string) []byte {
	return secretHash(token)
}
