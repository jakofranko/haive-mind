// Command fakeagent stands in for `opencode run <prompt>` in end-to-end tests.
// It prints AGENTREPLY-<first word of the prompt> so a test can tell which
// prompt the app sent without depending on the real agent.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	prompt := strings.Join(os.Args[2:], " ") // os.Args[1] is "run"
	first := strings.Fields(prompt)
	word := ""
	if len(first) > 0 {
		word = first[0]
	}
	if path := os.Getenv("FAKE_AGENT_LOG"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "%q\n", prompt) // quoted: one record per line even for multi-line prompts
			f.Close()
		}
	}
	fmt.Print("AGENTREPLY-" + word)
}
