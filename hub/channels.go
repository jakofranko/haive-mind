package hub

import (
	"context"
	"os/exec"
	"time"
)

// Hub defines the interface for the hub channels
type Hub interface {
	Inbound() chan *Message
	Outbound() chan *Message
	AggregateChan() chan int
	SendChan() chan *Message
	NewRoundChan() chan int
	ExportChan() chan string
	ParticipantUpdateChan() chan Participant
	RoundChangedChan() chan int
	ExportCompleteChan() chan string
	ErrorChan() chan error
	SendToAgent(ctx context.Context, prompt string) (string, error)
}

// HubChannels provides typed channels for communication between TUI and hub
type HubChannels struct {
	inbound               chan *Message
	outbound              chan *Message
	aggregateChan         chan int
	sendChan              chan *Message
	newRoundChan          chan int
	exportChan            chan string
	participantUpdateChan chan Participant
	roundChangedChan      chan int
	exportCompleteChan    chan string
	errorChan             chan error
	agentRunner           AgentRunner
}

func NewHubChannels() *HubChannels {
	return &HubChannels{
		inbound:               make(chan *Message, 100),
		outbound:              make(chan *Message, 100),
		aggregateChan:         make(chan int, 10),
		sendChan:              make(chan *Message, 10),
		newRoundChan:          make(chan int, 10),
		exportChan:            make(chan string, 10),
		participantUpdateChan: make(chan Participant, 100),
		roundChangedChan:      make(chan int, 10),
		exportCompleteChan:    make(chan string, 10),
		errorChan:             make(chan error, 10),
		agentRunner:           newDefaultAgentRunner(),
	}
}

func (h *HubChannels) Inbound() chan *Message                  { return h.inbound }
func (h *HubChannels) Outbound() chan *Message                 { return h.outbound }
func (h *HubChannels) AggregateChan() chan int                 { return h.aggregateChan }
func (h *HubChannels) SendChan() chan *Message                 { return h.sendChan }
func (h *HubChannels) NewRoundChan() chan int                  { return h.newRoundChan }
func (h *HubChannels) ExportChan() chan string                 { return h.exportChan }
func (h *HubChannels) ParticipantUpdateChan() chan Participant { return h.participantUpdateChan }
func (h *HubChannels) RoundChangedChan() chan int              { return h.roundChangedChan }
func (h *HubChannels) ExportCompleteChan() chan string         { return h.exportCompleteChan }
func (h *HubChannels) ErrorChan() chan error                   { return h.errorChan }

const defaultAgentTimeout = 120 * time.Second

// AgentRunner defines the interface for running an agent subprocess
type AgentRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// defaultAgentRunner runs `opencode run` subprocess
type defaultAgentRunner struct {
	timeout time.Duration
}

func newDefaultAgentRunner() *defaultAgentRunner {
	return &defaultAgentRunner{timeout: defaultAgentTimeout}
}

func (r *defaultAgentRunner) Run(ctx context.Context, prompt string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "opencode", "run", prompt)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return "", ErrAgentTimeout
		}
		return "", err
	}
	return string(output), nil
}

// ErrAgentTimeout is returned when the agent subprocess times out
var ErrAgentTimeout = &AgentError{code: "timeout", message: "agent timed out"}

// AgentError represents an error from the agent subprocess
type AgentError struct {
	code    string
	message string
}

func (e *AgentError) Error() string {
	return e.message
}

func (e *AgentError) Code() string {
	return e.code
}

// SendToAgent runs the agent subprocess and returns its output
func (h *HubChannels) SendToAgent(ctx context.Context, prompt string) (string, error) {
	return h.agentRunner.Run(ctx, prompt)
}

// SetAgentRunner sets a custom agent runner (for testing)
func (h *HubChannels) SetAgentRunner(runner AgentRunner) {
	h.agentRunner = runner
}
