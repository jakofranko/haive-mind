package coordinator_test

import (
	"context"
	"sync"
	"testing"

	"jakofranko/haive-mind/hub"
)

// snapshots collects the Transcript snapshot messages published on the hub.
type snapshots struct {
	mu   sync.Mutex
	msgs []*hub.Message
}

func watchSnapshots(t *testing.T, h hub.Hub) *snapshots {
	t.Helper()
	s := &snapshots{}
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeTranscriptSnapshot, func(m *hub.Message) error {
		s.mu.Lock()
		s.msgs = append(s.msgs, m)
		s.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

// last returns the newest snapshot addressed to id, or nil.
func (s *snapshots) last(id string) *hub.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.msgs) - 1; i >= 0; i-- {
		if s.msgs[i].To == id {
			return s.msgs[i]
		}
	}
	return nil
}

func (s *snapshots) await(t *testing.T, id string) (transcript []*hub.Message, lastSubmission *hub.Message) {
	t.Helper()
	eventually(t, "snapshot for "+id, func() bool { return s.last(id) != nil })
	transcript, lastSubmission, err := s.last(id).TranscriptSnapshot()
	if err != nil {
		t.Fatalf("snapshot content is not a snapshot: %v", err)
	}
	return transcript, lastSubmission
}

func TestSnapshot_SentToConnectingParticipantWithTranscript(t *testing.T) {
	h, c, _ := newRig(t)
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeDirective, "Host", "what now?", 1))
	eventually(t, "directive recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeDirective, "") == 1
	})
	s := watchSnapshots(t, h)

	id := hub.ParticipantID("bob")
	connect(t, h, id)

	transcript, _ := s.await(t, id)
	if countType(transcript, hub.MessageTypeDirective, "") != 1 {
		t.Errorf("snapshot has %d directives, want 1", countType(transcript, hub.MessageTypeDirective, ""))
	}
}

func submit(h hub.Hub, id, content string, round int) {
	m := hub.NewMessage(hub.MessageTypeResponse, id, content, round)
	m.FromID = id
	h.Send(context.Background(), m)
}

func TestSnapshot_CarriesOnlyTheAddresseesLastSubmission(t *testing.T) {
	h, c, _ := newRig(t)
	bob, alice := hub.ParticipantID("bob"), hub.ParticipantID("alice")
	submit(h, bob, "first", 1)
	submit(h, bob, "second", 1)
	submit(h, alice, "alice's", 1)
	eventually(t, "responses tracked", func() bool {
		return c.LastSubmission(bob) != nil && c.LastSubmission(bob).Content == "second" && c.LastSubmission(alice) != nil
	})
	s := watchSnapshots(t, h)

	connect(t, h, bob)

	_, last := s.await(t, bob)
	if last == nil || last.Content != "second" {
		t.Fatalf("last submission = %+v, want bob's latest (second)", last)
	}
	if last.FromID != bob {
		t.Errorf("last submission is from %q, want %q", last.FromID, bob)
	}
}

func TestSnapshot_NoLastSubmissionAfterRoundChange(t *testing.T) {
	h, c, _ := newRig(t)
	bob := hub.ParticipantID("bob")
	submit(h, bob, "old answer", 1)
	eventually(t, "response tracked", func() bool { return c.LastSubmission(bob) != nil })
	h.StartRound(1)
	eventually(t, "round 2", func() bool { return c.Round() == 2 })
	s := watchSnapshots(t, h)

	connect(t, h, bob)

	if _, last := s.await(t, bob); last != nil {
		t.Errorf("last submission = %+v after a round change, want none", last)
	}
}

func TestSnapshot_HostHasNoLastSubmission(t *testing.T) {
	h, c, _ := newRig(t)
	bob := hub.ParticipantID("bob")
	submit(h, bob, "bob's", 1)
	eventually(t, "response tracked", func() bool { return c.LastSubmission(bob) != nil })
	s := watchSnapshots(t, h)

	connect(t, h, hub.HostID)

	if _, last := s.await(t, hub.HostID); last != nil {
		t.Errorf("host snapshot has last submission %+v, want none", last)
	}
}
