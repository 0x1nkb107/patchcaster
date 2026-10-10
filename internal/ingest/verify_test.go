package ingest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vector body/secret/id/timestamp are the Standard Webhooks reference
// sample; the signature was produced by openssl(1), not by this codebase, so
// the test pins the wire format rather than agreeing with itself.
const (
	vecSecret = "kms"
	vecID     = "res_1HeHu9dvk0If7e5teq61vf9t"
	vecTS     = "1615420300"
	vecBody   = `{"event":"account.created","account":"acct_1HeHu8stYWDSwerW8Ttux3e4"}`
	vecSig    = "v1,SkXpc9IRRDdaIquXTdIyz2BBsQ1Wqn7wr8fQH3s8X0o="
)

func TestVerify_ReferenceVector(t *testing.T) {
	// arrange
	secrets := [][]byte{[]byte(vecSecret)}

	// act
	ok := Verify(secrets, vecID, vecTS, []byte(vecBody), vecSig)

	// assert
	require.True(t, ok, "reference signature must verify")
}

func signFor(t *testing.T, secret, id, ts string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(id + "." + ts + "." + string(body)))
	require.NoError(t, err)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerify_Decisions(t *testing.T) {
	body := []byte(`{"object_kind":"merge_request"}`)
	id, ts := "msg_1", "1700000000"
	good := signFor(t, "s3cret", id, ts, body)

	tests := []struct {
		name    string
		secrets [][]byte
		id, ts  string
		sig     string
		want    bool
	}{
		{"valid", [][]byte{[]byte("s3cret")}, id, ts, good, true},
		{"wrong secret", [][]byte{[]byte("other")}, id, ts, good, false},
		{"no secrets configured", nil, id, ts, good, false},
		{"missing id", [][]byte{[]byte("s3cret")}, "", ts, good, false},
		{"missing timestamp", [][]byte{[]byte("s3cret")}, id, "", good, false},
		{"missing header", [][]byte{[]byte("s3cret")}, id, ts, "", false},
		{"foreign scheme only", [][]byte{[]byte("s3cret")}, id, ts, "v0,abcd", false},
		{"multiple candidates one valid", [][]byte{[]byte("s3cret")}, id, ts, "v1,Zm9v " + good, true},
		{"rotation second secret", [][]byte{[]byte("old"), []byte("s3cret")}, id, ts, good, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// act
			got := Verify(tc.secrets, tc.id, tc.ts, body, tc.sig)

			// assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestVerify_TamperedBodyFails(t *testing.T) {
	// arrange: signature computed over the original body.
	body := []byte(`{"object_kind":"merge_request"}`)
	sig := signFor(t, "s3cret", "msg_1", "1700000000", body)

	// act: same headers, mutated body.
	ok := Verify([][]byte{[]byte("s3cret")}, "msg_1", "1700000000", []byte(`{"object_kind":"push"}`), sig)

	// assert
	assert.False(t, ok)
}
