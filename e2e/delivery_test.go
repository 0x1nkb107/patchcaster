package e2e

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstMR is the merge request the scenarios open and comment on.
var firstMR = mergeRequest{id: 101, iid: 7}

// SCEN-001 with the wake-up path of ADR-019: an hour-long poll leaves the
// commit notification as the only way the delivery can start.
func TestOpeningAMergeRequestStartsAThread(t *testing.T) {
	// arrange: wait for the listener, so that its catch-up wake-up has passed
	// before the delivery commits.
	db := freshDatabase(t)
	mattermost := newFakeMattermost(t)
	svc := startService(t, serviceConfig{mattermostURL: mattermost.url(), pollInterval: time.Hour})
	eventually(t, 10*time.Second, "the wake-up listener", svc.listening)

	// act
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))

	// assert
	eventually(t, 10*time.Second, "the root post", func() bool {
		return len(mattermost.acceptedPosts()) == 1
	})
	root := mattermost.acceptedPosts()[0]
	assert.Equal(t, channelID, root.ChannelID, "the routing table picks the channel")
	assert.Empty(t, root.RootID, "a root post starts a thread")
	eventually(t, 5*time.Second, "the delivery to settle", func() bool {
		return assert.ObjectsAreEqual([]string{"sent"}, outboxStates(t, db))
	})
	assert.Equal(t, root.ID, threadRoot(t, db, firstMR.ref()), "the thread is mapped to the root post")
}

// SCEN-002: later activity replies in the thread and refreshes its root.
func TestACommentRepliesInTheThread(t *testing.T) {
	// arrange: an announced merge request.
	freshDatabase(t)
	mattermost := newFakeMattermost(t)
	svc := startService(t, serviceConfig{mattermostURL: mattermost.url()})
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	eventually(t, 10*time.Second, "the root post", func() bool {
		return len(mattermost.acceptedPosts()) == 1
	})
	root := mattermost.acceptedPosts()[0]

	// act
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, commentCreated(t, firstMR, 501)))

	// assert
	eventually(t, 10*time.Second, "the reply", func() bool {
		return len(mattermost.acceptedPosts()) == 2
	})
	reply := mattermost.acceptedPosts()[1]
	assert.Equal(t, root.ID, reply.RootID, "the comment replies in the thread")
	assert.Equal(t, channelID, reply.ChannelID)
	eventually(t, 5*time.Second, "the root refresh", func() bool {
		return assert.ObjectsAreEqual([]string{root.ID}, mattermost.updatedPosts())
	})
}

// SCEN forged-replayed-rejected: a delivery with a wrong signature is
// refused, and a replayed delivery id queues nothing a second time.
func TestForgedAndReplayedDeliveriesQueueNothing(t *testing.T) {
	// arrange
	db := freshDatabase(t)
	mattermost := newFakeMattermost(t)
	svc := startService(t, serviceConfig{mattermostURL: mattermost.url()})
	body := mergeRequestOpened(t, firstMR)
	id := newDeliveryID(t)

	// act
	forged := svc.gitlab.deliverSigned(t, newDeliveryID(t), body, "not-the-secret")
	first := svc.gitlab.deliverAs(t, id, body)
	replayed := svc.gitlab.deliverAs(t, id, body)

	// assert
	assert.Equal(t, http.StatusUnauthorized, forged, "a forged delivery is refused")
	assert.Equal(t, http.StatusOK, first)
	assert.Equal(t, http.StatusOK, replayed, "a replay is acknowledged, so GitLab stops retrying")
	assert.Equal(t, 1, seenWebhooks(t, db), "only the genuine delivery is recorded")
	assert.Len(t, outboxRows(t, db), 1, "the replay queues no second message")
	eventually(t, 10*time.Second, "the single root post", func() bool {
		return len(mattermost.acceptedPosts()) == 1
	})
}

// SCEN-005: during an outage the root retries and the comment waits behind
// it untouched. After recovery both post, in order, and nothing is skipped.
func TestAnOutageDelaysTheThreadButKeepsItInOrder(t *testing.T) {
	// arrange: Mattermost answers 503 to every post.
	db := freshDatabase(t)
	mattermost := newFakeMattermost(t)
	mattermost.setFailing(true)
	svc := startService(t, serviceConfig{mattermostURL: mattermost.url()})
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, commentCreated(t, firstMR, 501)))
	eventually(t, 10*time.Second, "the root post to be retried", func() bool {
		return mattermost.attemptCount() >= 2
	})
	rows := outboxRows(t, db)
	require.Len(t, rows, 2)
	assert.Positive(t, rows[0].attempts, "the root has failed attempts")
	assert.Zero(t, rows[1].attempts, "the comment waits behind its root, unattempted")
	assert.Equal(t, "pending", rows[1].state)
	assert.Empty(t, mattermost.acceptedPosts())

	// act: Mattermost recovers.
	mattermost.setFailing(false)

	// assert
	eventually(t, 20*time.Second, "the root and its reply", func() bool {
		return len(mattermost.acceptedPosts()) == 2
	})
	posts := mattermost.acceptedPosts()
	assert.Empty(t, posts[0].RootID, "the root posts first")
	assert.Equal(t, posts[0].ID, posts[1].RootID, "the comment replies to it")
	eventually(t, 5*time.Second, "both deliveries to settle as sent", func() bool {
		return assert.ObjectsAreEqual([]string{"sent", "sent"}, outboxStates(t, db))
	})
}

// SYSREQ-017: a delivery that was acknowledged survives SIGKILL in the middle
// of its post and is posted exactly once by the next process, after the
// killed process's claim lease runs out.
func TestAnAcceptedDeliverySurvivesAKill(t *testing.T) {
	// arrange: the root post is in flight when the process dies.
	db := freshDatabase(t)
	mattermost := newFakeMattermost(t)
	mattermost.holdPosts()
	cfg := serviceConfig{mattermostURL: mattermost.url()}
	first := startService(t, cfg)
	require.Equal(t, http.StatusOK, first.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	mattermost.awaitHeld(t)

	first.signal(t, syscall.SIGKILL)
	first.awaitExit(t, 5*time.Second)
	eventually(t, 5*time.Second, "the held post to be abandoned", func() bool {
		return mattermost.abandonedCount() == 1
	})
	require.Empty(t, mattermost.acceptedPosts(), "the killed process got no answer")
	require.Equal(t, []string{"claimed"}, outboxStates(t, db), "the delivery stays claimed under its lease")
	mattermost.release()

	// act
	startService(t, cfg)

	// assert
	eventually(t, 15*time.Second, "the redelivered root post", func() bool {
		return len(mattermost.acceptedPosts()) == 1
	})
	eventually(t, 5*time.Second, "the delivery to settle as sent", func() bool {
		return assert.ObjectsAreEqual([]string{"sent"}, outboxStates(t, db))
	})
	assert.Len(t, mattermost.acceptedPosts(), 1, "the message is posted exactly once")
}

// SYSREQ-023: SIGTERM stops new webhooks at once but lets the delivery in
// hand finish. Readiness reports the drain, and the process exits cleanly.
func TestSIGTERMLetsTheDeliveryInHandFinish(t *testing.T) {
	// arrange: the root post is in flight when the signal arrives.
	db := freshDatabase(t)
	mattermost := newFakeMattermost(t)
	mattermost.holdPosts()
	svc := startService(t, serviceConfig{mattermostURL: mattermost.url()})
	require.Equal(t, http.StatusOK, svc.gitlab.deliver(t, mergeRequestOpened(t, firstMR)))
	mattermost.awaitHeld(t)

	// act
	svc.signal(t, syscall.SIGTERM)

	// assert: during the drain readiness fails and webhooks are refused.
	eventually(t, 5*time.Second, "readiness to report the drain", func() bool {
		return svc.readiness() == http.StatusServiceUnavailable
	})
	eventually(t, 5*time.Second, "the webhook listener to close", func() bool {
		return !svc.acceptsConnections()
	})

	// assert: the delivery in hand finishes once Mattermost answers.
	mattermost.release()
	assert.Equal(t, 0, svc.awaitExit(t, 15*time.Second), "a drained shutdown exits cleanly")
	assert.Len(t, mattermost.acceptedPosts(), 1, "the delivery in hand was posted")
	assert.Zero(t, mattermost.abandonedCount(), "the drain did not cut the post off")
	assert.Equal(t, []string{"sent"}, outboxStates(t, db))
	assert.NotEmpty(t, threadRoot(t, db, firstMR.ref()), "the thread was mapped before exit")
}
