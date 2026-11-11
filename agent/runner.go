package agent

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner runs one prompt and returns the answer. Run must return promptly
// when ctx is cancelled: the hub cancels it on Stop and then waits for every
// run in flight to finish.
type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

const defaultAgentTimeout = 120 * time.Second

// outputWaitDelay is how long Run still waits for output once opencode is killed.
const outputWaitDelay = time.Second

type defaultRunner struct {
	timeout time.Duration
}

func NewDefaultRunner() *defaultRunner {
	return NewRunner(defaultAgentTimeout)
}

// NewRunner returns a runner that gives the agent up to timeout to answer.
func NewRunner(timeout time.Duration) *defaultRunner {
	return &defaultRunner{timeout: timeout}
}

func (r *defaultRunner) Run(ctx context.Context, prompt string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// The prompt goes on stdin, not as an argument: on Windows opencode is
	// npm's opencode.cmd, and a batch file cuts an argument at its first
	// newline, so a multi-line prompt (the synthesis prompt) arrived as just
	// its first line.
	cmd := exec.CommandContext(cmdCtx, "opencode", "run")
	cmd.Stdin = strings.NewReader(prompt)
	// opencode may run as a wrapper around a child (npm's opencode.cmd starts
	// opencode.exe), so a timeout or cancel kills the whole tree; and Run stops
	// waiting for output soon after, should anything still hold stdout open.
	killTreeOnCancel(cmd)
	cmd.WaitDelay = outputWaitDelay
	output, err := cmd.Output() // stdout only: stderr is diagnostics, not the answer
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return "", ErrAgentTimeout
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				return "", fmt.Errorf("%w: %s", err, stderr)
			}
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
