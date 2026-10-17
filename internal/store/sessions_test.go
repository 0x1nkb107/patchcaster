package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionsExpire(t *testing.T) {
	// arrange
	st, _ := openTestStore(t)
	ctx := context.Background()
	sessions := st.Sessions()
	now := time.Now().UTC()
	require.NoError(t, sessions.CommitCtx(ctx, "live", []byte("data"), now.Add(time.Hour)))
	require.NoError(t, sessions.CommitCtx(ctx, "gone", []byte("data"), now.Add(-time.Second)))

	// act
	liveData, liveFound, liveErr := sessions.FindCtx(ctx, "live")
	_, goneFound, _ := sessions.FindCtx(ctx, "gone")
	require.NoError(t, sessions.DeleteCtx(ctx, "live"))
	_, afterDelete, _ := sessions.FindCtx(ctx, "live")
	require.NoError(t, st.DeleteExpiredSessions(ctx, now))

	// assert
	require.NoError(t, liveErr)
	assert.True(t, liveFound)
	assert.Equal(t, []byte("data"), liveData)
	assert.False(t, goneFound, "an expired session is not found")
	assert.False(t, afterDelete)
}
