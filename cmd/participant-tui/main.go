package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/participant"
)

func main() {
	yourName := "Agent-1"
	sshUsername := "user1"
	if len(os.Args) > 1 {
		yourName = os.Args[1]
	}
	if len(os.Args) > 2 {
		sshUsername = os.Args[2]
	}

	hubChans := hub.NewHubChannels()
	model := participant.NewParticipantModel(hubChans, yourName, sshUsername)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}