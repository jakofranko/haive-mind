package coordinator_test

import (
	"strings"
	"testing"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
)

// The Coordinator's state behaviour, tested through its synchronous core: each
// message is applied in turn and the test reads what Apply returns.

func directive(text string, round int) *hub.Message {
	return hub.NewMessage(hub.MessageTypeDirective, "Host", text, round)
}

func newRound(round int) *hub.Message {
	return hub.NewMessage(hub.MessageTypeNewRound, "hub", "", round)
}

func aggregateRequest(round int) *hub.Message {
	return hub.NewMessage(hub.MessageTypeAggregateRequest, "hub", "", round)
}

// participantEvent is the hub's announcement for participant id, as it
// broadcasts one: the id in ID and the event in Content.
func participantEvent(id, event string) *hub.Message {
	m := hub.NewMessage(hub.MessageTypeParticipantEvent, "hub", event, 0)
	m.ID = id
	return m
}

func applyAll(s *coordinator.State, msgs ...*hub.Message) []*hub.Message {
	var out []*hub.Message
	for _, m := range msgs {
		out = append(out, s.Apply(m)...)
	}
	return out
}

func ofType(msgs []*hub.Message, typ hub.MessageType) []*hub.Message {
	var out []*hub.Message
	for _, m := range msgs {
		if m.Type == typ {
			out = append(out, m)
		}
	}
	return out
}

func roundStateOf(t *testing.T, m *hub.Message) (int, string) {
	t.Helper()
	if m.Type != hub.MessageTypeRoundState {
		t.Fatalf("got %s, want a round state", m.Type)
	}
	round, ids, err := m.RoundState()
	if err != nil {
		t.Fatalf("round state content %q: %v", m.Content, err)
	}
	return round, strings.Join(ids, ",")
}

func wantRoundState(t *testing.T, out []*hub.Message, round int, flagged string) {
	t.Helper()
	if len(out) != 1 {
		t.Fatalf("published %d messages, want one round state", len(out))
	}
	gotRound, gotFlagged := roundStateOf(t, out[0])
	if gotRound != round || gotFlagged != flagged {
		t.Errorf("round state = round %d flagged [%s], want round %d flagged [%s]", gotRound, gotFlagged, round, flagged)
	}
}

func wantNothing(t *testing.T, out []*hub.Message) {
	t.Helper()
	if len(out) != 0 {
		t.Errorf("published %d messages (first %s), want none", len(out), out[0].Type)
	}
}

// G12: the host flags a participant and everyone is told who is flagged.
func TestState_FlagAndUnflagAnnounceWhoIsFlagged(t *testing.T) {
	s := coordinator.NewState()

	wantRoundState(t, s.Apply(hub.NewFlag(1, "Agent-alice")), 1, "Agent-alice")
	wantRoundState(t, s.Apply(hub.NewUnflag(1, "Agent-alice")), 1, "")
	if got := s.Flagged(); len(got) != 0 {
		t.Errorf("Flagged() = %v, want none", got)
	}
}

func TestState_FlaggingTwiceAnnouncesOnce(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(hub.NewFlag(1, "Agent-alice"))

	wantNothing(t, s.Apply(hub.NewFlag(1, "Agent-alice")))
}

func TestState_UnflaggingSomeoneNotFlaggedAnnouncesNothing(t *testing.T) {
	s := coordinator.NewState()

	wantNothing(t, s.Apply(hub.NewUnflag(1, "Agent-alice")))
}

// A flagged participant who resubmits is no longer flagged, and everyone is
// told; anyone else stays flagged.
func TestState_ResubmitLiftsOnlyThatFlag(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, hub.NewFlag(1, "Agent-alice"), hub.NewFlag(1, "Agent-bob"))

	wantRoundState(t, s.Apply(response("Agent-alice", "Alice", "revised", 1)), 1, "Agent-bob")
}

// A response from someone not flagged announces nothing.
func TestState_ResponseFromUnflaggedParticipantAnnouncesNothing(t *testing.T) {
	s := coordinator.NewState()

	wantNothing(t, s.Apply(response("Agent-alice", "Alice", "answer", 1)))
}

// Applied in the order sent, a flag after a response stays raised.
func TestState_FlagAfterResponseStaysRaised(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, response("Agent-alice", "Alice", "answer", 1), hub.NewFlag(1, "Agent-alice"))

	if got := strings.Join(s.Flagged(), ","); got != "Agent-alice" {
		t.Errorf("Flagged() = [%s], want [Agent-alice]", got)
	}
}

func TestState_ResolveClearsEveryFlag(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, hub.NewFlag(1, "Agent-alice"), hub.NewFlag(1, "Agent-bob"))

	wantRoundState(t, s.Apply(hub.NewResolve(1)), 1, "")
	wantNothing(t, s.Apply(hub.NewResolve(1)))
}

// A new round advances the round, ends its flags, and announces both the Round
// state and the Round change, in that order.
func TestState_NewRoundAdvancesClearsFlagsAndAnnounces(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(hub.NewFlag(1, "Agent-alice"))

	out := s.Apply(newRound(1))

	if s.Round() != 2 {
		t.Errorf("Round() = %d, want 2", s.Round())
	}
	if len(out) != 2 {
		t.Fatalf("published %d messages, want a round state and a round change", len(out))
	}
	if round, flagged := roundStateOf(t, out[0]); round != 2 || flagged != "" {
		t.Errorf("round state = round %d flagged [%s], want round 2 with nobody flagged", round, flagged)
	}
	if out[1].Type != hub.MessageTypeRoundChange || out[1].Round != 2 {
		t.Errorf("second message = %s round %d, want round change to 2", out[1].Type, out[1].Round)
	}
}

// A request naming a round other than the current one is stale or a repeat:
// nothing is announced and the round does not move.
func TestState_StaleOrRepeatedNewRoundIsIgnored(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(newRound(1))

	for _, r := range []int{1, 0, 5} {
		wantNothing(t, s.Apply(newRound(r)))
	}
	if s.Round() != 2 {
		t.Errorf("Round() = %d, want 2", s.Round())
	}

	s.Apply(newRound(2)) // naming the current round still advances
	if s.Round() != 3 {
		t.Errorf("Round() = %d, want 3", s.Round())
	}
}

func TestState_LastSubmissionIsTheLatestThisRound(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		response("Agent-bob", "Bob", "first", 1),
		response("Agent-bob", "Bob", "second", 1),
		response("Agent-alice", "Alice", "alice's", 1),
	)

	if last := s.LastSubmission("Agent-bob"); last == nil || last.Content != "second" {
		t.Errorf("LastSubmission(bob) = %+v, want second", last)
	}
	if last := s.LastSubmission("Agent-carol"); last != nil {
		t.Errorf("LastSubmission(carol) = %+v, want none", last)
	}

	s.Apply(newRound(1))
	if last := s.LastSubmission("Agent-bob"); last != nil {
		t.Errorf("LastSubmission(bob) = %+v after a new round, want none", last)
	}
}

// The transcript records what is shown plus round and export events, in the
// order applied; signals and coordinator output are not recorded.
func TestState_TranscriptRecordsInOrder(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		directive("q", 1),
		hub.NewFlag(1, "Agent-alice"),
		response("Agent-alice", "Alice", "a", 1),
		aggregateRequest(1),
		hub.NewMessage(hub.MessageTypeExportComplete, "hub", "done", 1),
	)

	var got []string
	for _, m := range s.Transcript() {
		got = append(got, string(m.Type))
	}
	if want := "directive,response,export_complete"; strings.Join(got, ",") != want {
		t.Errorf("transcript types = %v, want %s", got, want)
	}
}

// snapshotFor connects id and returns the Transcript snapshot published for it.
func snapshotFor(t *testing.T, s *coordinator.State, id string) (transcript []*hub.Message, last *hub.Message) {
	t.Helper()
	out := s.Apply(participantEvent(id, hub.ParticipantEventConnect))
	snaps := ofType(out, hub.MessageTypeTranscriptSnapshot)
	if len(snaps) != 1 {
		t.Fatalf("published %d snapshots on connect, want 1", len(snaps))
	}
	if snaps[0].To != id {
		t.Errorf("snapshot addressed to %q, want %q", snaps[0].To, id)
	}
	transcript, last, err := snaps[0].TranscriptSnapshot()
	if err != nil {
		t.Fatalf("snapshot content: %v", err)
	}
	return transcript, last
}

// Someone connecting is told the current round and who is flagged, then sent
// the conversation so far.
func TestState_ConnectAnnouncesRoundStateThenSnapshot(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, newRound(1), hub.NewFlag(2, "Agent-alice"))

	out := s.Apply(participantEvent("Agent-bob", hub.ParticipantEventConnect))

	if len(out) != 2 || out[1].Type != hub.MessageTypeTranscriptSnapshot {
		t.Fatalf("published %d messages, want a round state then a snapshot", len(out))
	}
	if round, flagged := roundStateOf(t, out[0]); round != 2 || flagged != "Agent-alice" {
		t.Errorf("round state = round %d flagged [%s], want round 2 flagged [Agent-alice]", round, flagged)
	}
}

// A repeated connect repeats the same state and changes nothing.
func TestState_RepeatedConnectChangesNothing(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(hub.NewFlag(1, "Agent-alice"))

	for i := 0; i < 2; i++ {
		out := s.Apply(participantEvent("Agent-bob", hub.ParticipantEventConnect))
		if round, flagged := roundStateOf(t, out[0]); round != 1 || flagged != "Agent-alice" {
			t.Errorf("connect %d: round state = round %d flagged [%s]", i+1, round, flagged)
		}
		if len(ofType(out, hub.MessageTypeRoundChange)) != 0 {
			t.Errorf("connect %d announced a round change", i+1)
		}
	}
	if s.Round() != 1 {
		t.Errorf("Round() = %d, want 1", s.Round())
	}
}

// Renaming, a status change and a disconnect are not arriving: nothing is
// announced.
func TestState_OtherParticipantEventsAnnounceNothing(t *testing.T) {
	s := coordinator.NewState()
	for _, ev := range []string{hub.ParticipantEventStatusChange, hub.ParticipantEventDisconnect} {
		wantNothing(t, s.Apply(participantEvent("Agent-bob", ev)))
	}
}

func TestState_SnapshotCarriesTheTranscript(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(directive("what now?", 1))

	transcript, _ := snapshotFor(t, s, "Agent-bob")

	if len(transcript) != 1 || transcript[0].Content != "what now?" {
		t.Errorf("snapshot transcript = %v, want the directive", transcript)
	}
}

// The snapshot carries only the addressee's last submission, and it is the
// same response the snapshot's transcript ends with for that participant.
func TestState_SnapshotLastSubmissionAgreesWithItsTranscript(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		response("Agent-bob", "Bob", "first", 1),
		response("Agent-bob", "Bob", "second", 1),
		response("Agent-alice", "Alice", "alice's", 1),
	)

	transcript, last := snapshotFor(t, s, "Agent-bob")

	if last == nil || last.Content != "second" || last.FromID != "Agent-bob" {
		t.Fatalf("last submission = %+v, want bob's second", last)
	}
	var bobsLatest *hub.Message
	for _, m := range transcript {
		if m.FromID == "Agent-bob" {
			bobsLatest = m
		}
	}
	if bobsLatest == nil || bobsLatest.ID != last.ID {
		t.Errorf("snapshot's transcript ends bob's responses with %+v, want the last submission", bobsLatest)
	}
}

func TestState_SnapshotHasNoLastSubmissionAfterRoundChange(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, response("Agent-bob", "Bob", "old", 1), newRound(1))

	if _, last := snapshotFor(t, s, "Agent-bob"); last != nil {
		t.Errorf("last submission = %+v after a round change, want none", last)
	}
}

func TestState_HostSnapshotHasNoLastSubmission(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(response("Agent-bob", "Bob", "bob's", 1))

	if _, last := snapshotFor(t, s, hub.HostID); last != nil {
		t.Errorf("host snapshot has last submission %+v, want none", last)
	}
}

// synthesisPrompt applies an aggregate request and returns the prompt asked of
// the synthesizer.
func synthesisPromptFor(t *testing.T, s *coordinator.State, round int) string {
	t.Helper()
	out := s.Apply(aggregateRequest(round))
	if len(out) != 1 || out[0].Type != hub.MessageTypeSynthesisRequest {
		t.Fatalf("published %v, want one synthesis request", out)
	}
	return out[0].Content
}

// G11: an aggregate request asks for one synthesis of the round's question and
// responses.
func TestState_AggregateRequestAsksForSynthesisOfQuestionAndResponses(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		directive("Which database?", 1),
		response("alice", "alice", "use postgres", 1),
		response("bob", "bob", "use sqlite", 1),
	)

	prompt := synthesisPromptFor(t, s, 1)

	for _, want := range []string{"Which database?", "alice: use postgres", "bob: use sqlite"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("synthesis prompt lacks %q: %s", want, prompt)
		}
	}
}

func TestState_AggregateRequestWithNoResponsesAsksForNothing(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(directive("q", 1))

	wantNothing(t, s.Apply(aggregateRequest(1)))
}

// Only each participant's latest submission in the round is synthesized; the
// earlier one stays in the transcript.
func TestState_SynthesisUsesLatestSubmissionPerParticipant(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		directive("Which database?", 1),
		response("Agent-alice", "Alice", "use postgres", 1),
		response("Agent-bob", "Bob", "use sqlite", 1),
		response("Agent-alice", "Alice", "actually use mysql", 1),
	)

	prompt := synthesisPromptFor(t, s, 1)

	if strings.Contains(prompt, "use postgres") {
		t.Errorf("superseded answer is in the prompt: %s", prompt)
	}
	for _, want := range []string{"actually use mysql", "use sqlite"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q: %s", want, prompt)
		}
	}
	if n := len(ofType(s.Transcript(), hub.MessageTypeResponse)); n != 3 {
		t.Errorf("transcript has %d responses, want all 3", n)
	}
}

// The host decides what counts as a conflict; the model is not asked to.
func TestState_SynthesisPromptDoesNotAskTheModelToFindDisagreements(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, directive("q", 1), response("Agent-alice", "Alice", "a", 1))

	if prompt := strings.ToLower(synthesisPromptFor(t, s, 1)); strings.Contains(prompt, "disagree") {
		t.Errorf("prompt still asks about disagreements: %s", prompt)
	}
}

// A participant whose Display name is "agent" is still a participant: their
// Response is synthesized. The real Agent Reply is left out.
func TestState_ParticipantNamedAgentIsSynthesized_AgentReplyIsNot(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s,
		directive("Which database?", 1),
		hub.NewMessage(hub.MessageTypeAgentReply, "agent", "agent says: hi", 1),
		hub.NewMessage(hub.MessageTypeResponse, "agent", "use duckdb", 1),
	)

	prompt := synthesisPromptFor(t, s, 1)

	if !strings.Contains(prompt, "use duckdb") {
		t.Errorf("synthesis prompt lacks the participant's response: %s", prompt)
	}
	if strings.Contains(prompt, "agent says") {
		t.Errorf("synthesis prompt includes the agent reply: %s", prompt)
	}
}

// A real Aggregate whose text happens to be "aggregate_request" is content for
// the agent, not a signal: it asks for no synthesis.
func TestState_AggregateWithSignalLookalikeTextAsksForNothing(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, directive("q", 1), response("alice", "alice", "use postgres", 1))

	wantNothing(t, s.Apply(hub.NewMessage(hub.MessageTypeAggregate, "Host", "aggregate_request", 1)))
}
