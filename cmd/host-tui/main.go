package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/host"
)

func main() {
	hubChans := hub.NewHub()
	model := host.NewAppModel(hubChans)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}
