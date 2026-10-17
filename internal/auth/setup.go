package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"net/url"
	"time"
)

// SetupLinkLifetime is how long a setup link works (SYSREQ-038).
const SetupLinkLifetime = time.Hour

// SetupLink is a new setup link and the hash under which it is stored.
type SetupLink struct {
	URL  string
	Hash []byte
}

// NewSetupLink returns a new setup link into the interface at interfaceURL.
// Its token travels in the URL fragment, which browsers never send to a
// server, so it stays out of access logs.
func NewSetupLink(interfaceURL *url.URL) SetupLink {
	token := rand.Text()
	link := interfaceURL.JoinPath("setup")
	link.Fragment = token
	return SetupLink{
		URL:  link.String(),
		Hash: SetupTokenHash(token),
	}
}

// SetupTokenHash returns the hash under which a setup link's token is
// stored. The database keeps only the hash, so its contents cannot open the
// setup.
func SetupTokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
