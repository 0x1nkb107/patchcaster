package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The key check is absent before the first start, keeps the first value
// stored, and changes only through a replace (SYSREQ-042).
func TestKeyCheckKeepsTheFirstAndReplaces(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// act
	before, beforeErr := st.KeyCheck(ctx)
	require.NoError(t, st.InitKeyCheck(ctx, []byte("first"), now))
	require.NoError(t, st.InitKeyCheck(ctx, []byte("second"), now))
	afterInit, _ := st.KeyCheck(ctx)
	require.NoError(t, st.ReplaceKeyCheck(ctx, []byte("resealed"), now))
	afterReplace, _ := st.KeyCheck(ctx)

	// assert
	require.NoError(t, beforeErr)
	assert.Nil(t, before)
	assert.Equal(t, []byte("first"), afterInit, "a racing second start must not overwrite the first check")
	assert.Equal(t, []byte("resealed"), afterReplace)
}
