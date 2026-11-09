package e2e

import "testing"

// #61: a first connect that picks a display name shows up in the host's list
// under that name once the prompt is answered.
func TestFirstConnectPickingAName_AppearsToTheHostUnderThatName(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")

	host.waitFor(t, "Alice  idle")
	alice.waitFor(t, "Alice  idle")
}
