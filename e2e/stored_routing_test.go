package e2e

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patchcaster-app/patchcaster/e2e/apiv1"
)

// secondMR is a merge request opened after a configuration change.
var secondMR = mergeRequest{id: 202, iid: 8}

// postsIn returns the accepted posts in channel, in arrival order.
func postsIn(m *fakeMattermost, channel string) []post {
	var in []post
	for _, p := range m.acceptedPosts() {
		if p.ChannelID == channel {
			in = append(in, p)
		}
	}
	return in
}

// A route added through the API applies to the next event, without a
// restart, and accepted events are not replayed into it (SYSREQ-034,
// SCEN-018). The event then fans out to both sinks (SYSREQ-046).
func TestARouteChangeAppliesToTheNextEvent(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	svc, r, _ := startRouted(t, serviceConfig{}, mattermost)
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	eventually(t, 10*time.Second, "the first root", func() bool {
		return len(mattermost.acceptedPosts()) == 1
	})

	// act
	r.addRoute(t, r.addSink(t, r.chat, "other channel", otherChannelID), nil)
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, secondMR)))

	// assert
	eventually(t, 10*time.Second, "a root in each channel", func() bool {
		return len(mattermost.acceptedPosts()) == 3
	})
	assert.Len(t, postsIn(mattermost, channelID), 2, "the first route keeps posting")
	other := postsIn(mattermost, otherChannelID)
	require.Len(t, other, 1, "the new route takes the next event only")
	assert.Empty(t, other[0].RootID)
}

// A route that mutes comments posts no comment reply, but its board shows
// the same state as an unmuted sink's (SYSREQ-047, SCEN-020).
func TestAMutedReplyLeavesTheBoardAlike(t *testing.T) {
	// arrange
	mattermost := newFakeMattermost(t)
	freshDatabase(t)
	svc, r, _ := startRouted(t, serviceConfig{}, mattermost)
	r.addRoute(t, r.addSink(t, r.chat, "quiet channel", otherChannelID), []apiv1.ReplyKind{apiv1.ReplyKindComment})
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	eventually(t, 10*time.Second, "a root in each channel", func() bool {
		return len(mattermost.acceptedPosts()) == 2
	})
	loud, quiet := postsIn(mattermost, channelID)[0], postsIn(mattermost, otherChannelID)[0]

	// act
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, commentCreated(t, firstMR, 501)))

	// assert
	eventually(t, 10*time.Second, "the reply and both refreshes", func() bool {
		updated := mattermost.updatedPosts()
		return len(postsIn(mattermost, channelID)) == 2 &&
			slices.Contains(updated, loud.ID) && slices.Contains(updated, quiet.ID)
	})
	assert.Equal(t, loud.ID, postsIn(mattermost, channelID)[1].RootID, "the unmuted sink replies")
	assert.Len(t, postsIn(mattermost, otherChannelID), 1, "the muted sink posts no reply")
}

// A sink that cannot deliver holds back only its own thread: the other
// sink's messages go out on time, and the failing one catches up in order
// once it recovers (SYSREQ-048, SCEN-019).
func TestAFailingSinkDelaysNoOther(t *testing.T) {
	// arrange
	healthy, failing := newFakeMattermost(t), newFakeMattermost(t)
	freshDatabase(t)
	svc, r, _ := startRouted(t, serviceConfig{}, healthy)
	second := connectChat(t, r.admin, "second mattermost", failing)
	r.addRoute(t, r.addSink(t, second, "second channel", channelID), nil)
	failing.setFailing(true)

	// act
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, commentCreated(t, firstMR, 501)))

	// assert: the healthy sink delivers both while the other keeps failing.
	eventually(t, 10*time.Second, "the healthy sink's root and reply", func() bool {
		return len(healthy.acceptedPosts()) == 2
	})
	eventually(t, 10*time.Second, "the failing sink to be retried", func() bool {
		return failing.attemptCount() >= 2
	})
	assert.Empty(t, failing.acceptedPosts())

	// act: the failing sink recovers.
	failing.setFailing(false)

	// assert
	eventually(t, 20*time.Second, "the recovered sink's root and reply", func() bool {
		return len(failing.acceptedPosts()) == 2
	})
	posts := failing.acceptedPosts()
	assert.Empty(t, posts[0].RootID, "the root posts first")
	assert.Equal(t, posts[0].ID, posts[1].RootID, "the comment replies to it")
}
