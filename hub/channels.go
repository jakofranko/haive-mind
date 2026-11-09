package hub

import (
	"context"
	"jakofranko/haive-mind/agent"
)

type Subscription interface {
	Unsubscribe()
}

type Hub interface {
	Send(ctx context.Context, msg *Message) error
	Subscribe(ctx context.Context, msgType MessageType, handler func(*Message) error) (Subscription, error)
	// SubscribeTypes delivers several message types to one handler in send order.
	SubscribeTypes(ctx context.Context, msgTypes []MessageType, handler func(*Message) error) (Subscription, error)
	RequestAggregate(round int) error
	// StartRound asks to end the given round and start the next. The round is
	// the one being ended; the Coordinator ignores it unless it is current.
	StartRound(round int) error
	Export(path string) error
	RegisterParticipant(p *Participant) error
	UpdateParticipantStatus(name string, status ParticipantStatus) error
	UnregisterParticipant(name string) error
	GetDisplayName(id string) string
	SetDisplayName(id, displayName string)
	SetDraft(name, text string)
	GetDraft(name string) string
	SetAgentRunner(runner agent.Runner)
	SetSynthesizer(runner agent.Runner)
	Start() error
	Stop() error
	GetParticipants() []*Participant
}
