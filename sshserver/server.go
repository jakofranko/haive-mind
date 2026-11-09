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

// NewServer builds the SSH server listening on addr (host:port; an empty host
// means every interface). Tests pass a loopback address so the OS firewall has
// nothing to prompt about.
func NewServer(h hub.Hub, addr, hostKey string) *ssh.Server {
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

	opts := []ssh.Option{
		wish.WithAddress(addr),
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
	model    interface{ Close() }
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

	var model interface {
		tea.Model
		Close()
	}
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

	program := tea.NewProgram(model, append(bubbletea.MakeOptions(s), tea.WithoutSignalHandler())...) // the server, not each session, handles Ctrl+C

	session := &trackedSession{
		program:  program,
		model:    model,
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

	go m.trackSession(s.Context(), session)

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

// trackSession cleans up after a session once its SSH connection ends. It
// waits on the SSH session's context, not on program.Wait(): Wait reads a
// channel that Run creates, so a Wait called before Run starts (as it is here,
// since the middleware runs the program after programHandler returns) blocks
// forever and the end of the session is never processed.
func (m *sessionManager) trackSession(ctx context.Context, session *trackedSession) {
	<-ctx.Done()
	session.program.Kill() // the middleware also quits it; Kill is a no-op once it has exited
	session.model.Close()

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
