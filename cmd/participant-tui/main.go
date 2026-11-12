package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/participant"
)

func main() {
	sshUsername := "user1"
	if len(os.Args) > 1 {
		sshUsername = os.Args[1]
	}

	hubChans := hub.NewHub()
	model := participant.NewParticipantModel(hubChans, hub.ParticipantID(sshUsername), sshUsername)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
