// Command fakeagent stands in for `opencode run` (prompt on stdin) in end-to-end tests.
// It prints AGENTREPLY-<first word of the prompt> so a test can tell which
// prompt the app sent without depending on the real agent.
//
// FAKE_AGENT_SLOW_PROMPT=word makes a call whose prompt starts with that word
// take FAKE_AGENT_SLOW_FOR (a duration, default 3s) before it answers, to
// exercise what people see while a call is pending.
//
// FAKE_AGENT_HANG_FIRST=N makes the first N calls hang (until killed), to
// exercise the agent timeout.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	in, _ := io.ReadAll(os.Stdin) // the prompt; os.Args[1] is "run"
	prompt := string(in)
	first := strings.Fields(prompt)
	word := ""
	if len(first) > 0 {
		word = first[0]
	}
	earlierCalls := 0
	if path := os.Getenv("FAKE_AGENT_LOG"); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			earlierCalls = strings.Count(string(b), "\n")
		}
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintf(f, "%q\n", prompt) // quoted: one record per line even for multi-line prompts
			f.Close()
		}
	}
	if n, _ := strconv.Atoi(os.Getenv("FAKE_AGENT_HANG_FIRST")); earlierCalls < n {
		time.Sleep(time.Minute)
	}
	if slow := os.Getenv("FAKE_AGENT_SLOW_PROMPT"); slow != "" && slow == word {
		d, err := time.ParseDuration(os.Getenv("FAKE_AGENT_SLOW_FOR"))
		if err != nil {
			d = 3 * time.Second
		}
		time.Sleep(d)
	}
	fmt.Print("AGENTREPLY-" + word)
}
