package agent

import (
	"context"
	"os/exec"
	"time"
)

type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

const defaultAgentTimeout = 120 * time.Second

type defaultRunner struct {
	timeout time.Duration
}

func NewDefaultRunner() *defaultRunner {
	return &defaultRunner{timeout: defaultAgentTimeout}
}

func (r *defaultRunner) Run(ctx context.Context, prompt string) (string, error) {
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

type AgentError struct {
	code    string
	message string
}

func NewAgentError(code, message string) *AgentError {
	return &AgentError{code: code, message: message}
}

func (e *AgentError) Error() string {
	return e.message
}

func (e *AgentError) Code() string {
	return e.code
}

var ErrAgentTimeout = NewAgentError("timeout", "agent timed out")