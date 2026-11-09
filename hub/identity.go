package hub

// HostID is the Participant id of the host.
const HostID = "host"

// ParticipantID is the one place that turns an SSH username into a
// Participant id.
func ParticipantID(sshUsername string) string {
	return "Agent-" + sshUsername
}
