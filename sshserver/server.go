package sshserver

import (
	"context"
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
		hub:                 h,
		participantSessions: make(map[string]*trackedSession),
	}

	// Start the hub
	if err := h.Start(); err != nil {
		log.Fatal(err)
	}

	// Subscribe to participant events
	ctx := context.Background()
	_, err := h.Subscribe(ctx, hub.MessageTypeParticipantEvent, func(msg *hub.Message) error {
		// Participant events are handled via RegisterParticipant/UnregisterParticipant
		// This subscription is for other components that need to react to participant changes
		return nil
	})
	if err != nil {
		log.Printf("Failed to subscribe to participant events: %v", err)
	}

	go mgr.broadcastLoop()

	opts := []ssh.Option{
		wish.WithAddress(":" + port),
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

type trackedSession struct {
	program  *tea.Program
	cancel   context.CancelFunc
	username string
	isHost   bool
	name     string
}

type sessionManager struct {
	mu                  sync.Mutex
	hostConnected       bool
	hostSession         *trackedSession
	participantSessions map[string]*trackedSession
	hub                 hub.Hub
}

func (m *sessionManager) programHandler(s ssh.Session) *tea.Program {
	m.mu.Lock()
	isHost := !m.hostConnected
	m.mu.Unlock()

	var model tea.Model
	var name string
	var sshUsername string

	if isHost {
		sshUsername = "host"
		name = "host"
		model = host.NewAppModel(m.hub)
	} else {
		sshUsername = s.User()
		name = "Agent-" + sshUsername
		model = participant.NewParticipantModel(m.hub, name, sshUsername)
	}

	ctx, cancel := context.WithCancel(context.Background())
	program := tea.NewProgram(model, bubbletea.MakeOptions(s)...)

	session := &trackedSession{
		program:  program,
		cancel:   cancel,
		username: sshUsername,
		isHost:   isHost,
		name:     name,
	}

	m.mu.Lock()
	if isHost {
		m.hostConnected = true
		m.hostSession = session
	} else {
		m.participantSessions[name] = session
	}
	m.mu.Unlock()

	go m.trackSession(ctx, session)

	// Register participant with hub
	displayName := m.hub.GetDisplayName(sshUsername)
	if displayName == "" {
		if isHost {
			displayName = "Host"
		} else {
			displayName = name
		}
	}

	p := hub.NewParticipantEvent(name, displayName, hub.StatusIdle, 0, hub.ParticipantEventConnect)
	if err := m.hub.RegisterParticipant(p); err != nil {
		log.Printf("Failed to register participant: %v", err)
	}

	return program
}

func (m *sessionManager) trackSession(ctx context.Context, session *trackedSession) {
	session.program.Wait()

	m.mu.Lock()
	if session.isHost {
		m.hostConnected = false
		m.hostSession = nil
	} else {
		delete(m.participantSessions, session.name)
	}
	m.mu.Unlock()

	// Unregister participant from hub
	if err := m.hub.UnregisterParticipant(session.name); err != nil {
		log.Printf("Failed to unregister participant: %v", err)
	}
}

func (m *sessionManager) broadcastLoop() {
	for msg := range m.hub.Outbound() {
		m.mu.Lock()
		programs := make([]*tea.Program, 0, len(m.participantSessions)+1)
		if m.hostSession != nil {
			programs = append(programs, m.hostSession.program)
		}
		for _, s := range m.participantSessions {
			programs = append(programs, s.program)
		}
		m.mu.Unlock()

		for _, p := range programs {
			p.Send(*msg)
		}
	}
}