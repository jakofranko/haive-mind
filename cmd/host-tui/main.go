package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/host"
)

func main() {
	hubChans := hub.NewHubChannels()
	model := host.NewAppModel(hubChans)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
