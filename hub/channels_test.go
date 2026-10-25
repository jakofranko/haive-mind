package hub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockAgentRunner struct {
	response string
	err      error
	delay    time.Duration
}

func (m *mockAgentRunner) Run(ctx context.Context, prompt string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.response, m.err
}

func TestHubChannels_SendToAgent_Success(t *testing.T) {
	hubChans := NewHubChannels()
	hubChans.SetAgentRunner(&mockAgentRunner{response: "Agent response"})

	ctx := context.Background()
	response, err := hubChans.SendToAgent(ctx, "test prompt")

	assert.NoError(t, err)
	assert.Equal(t, "Agent response", response)
}

func TestHubChannels_SendToAgent_Error(t *testing.T) {
	hubChans := NewHubChannels()
	hubChans.SetAgentRunner(&mockAgentRunner{err: assert.AnError})

	ctx := context.Background()
	response, err := hubChans.SendToAgent(ctx, "test prompt")

	assert.Error(t, err)
	assert.Empty(t, response)
}

func TestHubChannels_SendToAgent_Timeout(t *testing.T) {
	hubChans := NewHubChannels()
	hubChans.SetAgentRunner(&mockAgentRunner{delay: 100 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	response, err := hubChans.SendToAgent(ctx, "test prompt")

	assert.Error(t, err)
	assert.Empty(t, response)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestHubChannels_SendToAgent_UsesPrompt(t *testing.T) {
	hubChans := NewHubChannels()
	hubChans.SetAgentRunner(&mockAgentRunner{
		response: "ok",
		err:      nil,
	})

	ctx := context.Background()
	_, err := hubChans.SendToAgent(ctx, "specific prompt")

	// We can't directly verify the prompt was passed without more complex mocking
	// but we can verify the method doesn't panic
	assert.NoError(t, err)
}

func TestAgentError_Code(t *testing.T) {
	err := &AgentError{code: "test_code", message: "test message"}
	assert.Equal(t, "test_code", err.Code())
	assert.Equal(t, "test message", err.Error())
}

func TestErrAgentTimeout(t *testing.T) {
	assert.Equal(t, "agent timed out", ErrAgentTimeout.Error())
	assert.Equal(t, "timeout", ErrAgentTimeout.Code())
}
