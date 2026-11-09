package coordinator_test

import (
	"testing"
	"time"
)

// A first connect is a connect then a rename. Round state goes out for the
// connect only: renaming is not arriving.
func TestRoundState_PublishedOncePerConnectNotOnRename(t *testing.T) {
	h, _, _ := newRig(t)
	u := watchUpdates(t, h)

	connect(t, h, "Agent-bob")
	eventually(t, "round state on connect", func() bool { return u.count() == 1 })

	h.SetDisplayName("Agent-bob", "Bob")
	time.Sleep(100 * time.Millisecond) // room for an unwanted announcement

	if got := u.count(); got != 1 {
		t.Errorf("round state published %d times for one connect and a rename, want 1", got)
	}
}
