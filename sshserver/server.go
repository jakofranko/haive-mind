package sshserver

import (
	"log"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/host"
	"jakofranko/haive-mind/tui/participant"
)

func NewServer(h hub.Hub, port, hostKey string) *ssh.Server {
	mgr := &sessionManager{
		hub:                  h,
		participantPrograms: make(map[string]*tea.Program),
	}

	go mgr.broadcastLoop()

	opts := []ssh.Option{
		wish.WithAddress(":"+port),
		wish.WithMiddleware(
			logging.Middleware(),
			activeterm.Middleware(),
			bubbletea.MiddlewareWithProgramHandler(mgr.programHandler, termenv.Ascii),
		),
	}
	if hostKey != "" {
		opts = append(opts, wish.WithHostKeyPath(hostKey))
	}

	s, err := wish.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}
	return s
}

type sessionManager struct {
	mu                  sync.Mutex
	hostConnected       bool
	hostProgram         *tea.Program
	participantPrograms map[string]*tea.Program
	hub                 hub.Hub
}

func (m *sessionManager) programHandler(s ssh.Session) *tea.Program {
	m.mu.Lock()
	isHost := !m.hostConnected
	m.mu.Unlock()

	var model tea.Model
	var name string
	if isHost {
		model = host.NewAppModel(m.hub)
		name = "host"
	} else {
		sshUsername := s.User()
		name = "Agent-" + sshUsername
		model = participant.NewParticipantModel(m.hub, name, sshUsername)
	}

	program := tea.NewProgram(model, bubbletea.MakeOptions(s)...)

	m.mu.Lock()
	defer m.mu.Unlock()
	if isHost {
		m.hostConnected = true
		m.hostProgram = program
	} else {
		m.participantPrograms[name] = program
	}

	return program
}

func (m *sessionManager) broadcastLoop() {
	for msg := range m.hub.Outbound() {
		m.mu.Lock()
		programs := make([]*tea.Program, 0, len(m.participantPrograms)+1)
		if m.hostProgram != nil {
			programs = append(programs, m.hostProgram)
		}
		for _, p := range m.participantPrograms {
			programs = append(programs, p)
		}
		m.mu.Unlock()

		for _, p := range programs {
			p.Send(msg)
		}
	}
}