package sshserver

import (
	"log"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/logging"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/host"
	"jakofranko/haive-mind/tui/participant"
)

func NewServer(h hub.Hub, port, hostKey string) *ssh.Server {
	s, err := wish.NewServer(
		wish.WithAddress(":"+port),
		wish.WithHostKeyPath(hostKey),
		wish.WithMiddleware(
			logging.Middleware(),
			activeterm.Middleware(),
			middleware(h),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	return s
}

type sessionManager struct {
	mu           sync.Mutex
	hostConnected bool
	hostProgram   *tea.Program
	participantPrograms map[string]*tea.Program
	hub          hub.Hub
}

func middleware(h hub.Hub) wish.Middleware {
	mgr := &sessionManager{
		hub:                  h,
		participantPrograms: make(map[string]*tea.Program),
	}

	go mgr.broadcastLoop()

	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			mgr.mu.Lock()
			isHost := !mgr.hostConnected
			mgr.mu.Unlock()

			var p *tea.Program
			var name string
			if isHost {
				model := host.NewAppModel(h)
				p = tea.NewProgram(model, tea.WithAltScreen())
				name = "host"
				mgr.mu.Lock()
				mgr.hostConnected = true
				mgr.hostProgram = p
				mgr.mu.Unlock()
			} else {
				name = "Agent-" + s.User()
				model := participant.NewParticipantModel(h, name)
				p = tea.NewProgram(model, tea.WithAltScreen())
				mgr.mu.Lock()
				mgr.participantPrograms[name] = p
				mgr.mu.Unlock()
			}

			if err := p.Start(); err != nil {
				log.Printf("program error for %s: %v", name, err)
			}

			if isHost {
				mgr.mu.Lock()
				mgr.hostProgram = nil
				mgr.hostConnected = false
				mgr.mu.Unlock()
			} else {
				mgr.mu.Lock()
				delete(mgr.participantPrograms, name)
				mgr.mu.Unlock()
			}
		}
	}
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