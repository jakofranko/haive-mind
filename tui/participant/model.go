package participant

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/participant/response"
	"jakofranko/haive-mind/tui/participant/sidebar"
	"jakofranko/haive-mind/tui/participant/transcript"
	"jakofranko/haive-mind/tui/session"
)

const (
	paneTranscript = iota
	paneSidebar
	paneResponse
	paneNamePrompt
	numPanes
)

type ParticipantModel struct {
	hub                hub.Hub
	width              int
	height             int
	activePane         int
	round              int
	id                 string // stable hub identity (set by the SSH server); never changes
	displayName        string // what others see; chosen by the person
	lastSubmitted      string // what they last sent, offered back when the host asks for a revision
	lastSubmittedRound int    // the round lastSubmitted was sent in; it is offered back only in that round
	flaggedSelf        bool   // the host currently has this participant flagged
	status             hub.ParticipantStatus

	transcript transcript.Model
	sidebar    sidebar.Model
	response   response.Model
	namePrompt textinput.Model

	namePromptActive bool

	// The session's conversation with the hub.
	client *session.Client
}

func NewParticipantModel(hubChans hub.Hub, id, sshUsername string) *ParticipantModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &ParticipantModel{
		hub:              hubChans,
		activePane:       paneTranscript,
		round:            1,
		id:               id,
		status:           hub.StatusIdle,
		displayName:      id,
		transcript:       transcript.New(),
		sidebar:          sidebar.New(id),
		response:         response.New(),
		namePrompt:       namePrompt,
		namePromptActive: false,
		client:           session.New(hubChans, id),
	}

	// Pick up text a previous session left unsent.
	if draft := hubChans.GetDraft(id); draft != "" {
		m.response.SetDraft(draft)
	}

	m.sidebar.SetParticipants([]hub.Participant{
		{ID: m.id, DisplayName: id, Status: hub.StatusIdle},
	})

	// Check if we have a stored display name
	if displayName := hubChans.GetDisplayName(m.id); displayName != "" {
		m.displayName = displayName
		m.sidebar.SetParticipants([]hub.Participant{
			{ID: m.id, DisplayName: displayName, Status: hub.StatusIdle},
		})
	} else {
		// First time - activate name prompt
		m.activePane = paneNamePrompt
		m.namePromptActive = true
	}

	return m
}

func (m *ParticipantModel) Init() tea.Cmd {
	if m.namePromptActive {
		return tea.Batch(textinput.Blink, m.waitForEvent(), m.initialParticipants())
	}
	return tea.Batch(
		m.transcript.Init(),
		m.sidebar.Init(),
		m.response.Init(),
		m.waitForEvent(),
		m.initialParticipants(),
	)
}

type initialParticipantsMsg []hub.Participant

func (m *ParticipantModel) initialParticipants() tea.Cmd {
	return func() tea.Msg { return initialParticipantsMsg(m.client.Participants()) }
}

// eventMsg carries one event from the session client into Update.
type eventMsg struct{ session.Event }

// waitForEvent pops the client's next event; Update re-arms it. It returns
// nil once the client is closed, which ends the wait.
func (m *ParticipantModel) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := m.client.Next()
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

func (m *ParticipantModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		cmds = append(cmds, m.forwardToChildren(msg)...)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case eventMsg:
		m.reduce(msg.Event)
		return m, m.waitForEvent()

	case initialParticipantsMsg:
		m.sidebar.SetParticipants(msg)
		return m, nil
	}

	cmds = append(cmds, m.forwardToFocused(msg)...)
	return m, tea.Batch(cmds...)
}

// reduce applies one event from the session client to the panes.
func (m *ParticipantModel) reduce(ev session.Event) {
	switch ev := ev.(type) {
	case session.MessageEvent:
		m.transcript.AddMessage(ev.Message)
	case session.SnapshotEvent:
		m.transcript.PrependMessages(ev.Messages)
		if ev.LastSubmission != nil {
			m.lastSubmitted = ev.LastSubmission.Content
			m.lastSubmittedRound = ev.LastSubmission.Round
			// Round state arrives before the snapshot, so a participant who
			// reconnects already flagged learns of their last answer only now.
			if m.flaggedSelf {
				m.restoreLastSubmission()
			}
		}
	case session.ParticipantEvent:
		if ev.Participant.Status == hub.StatusDisconnected {
			m.sidebar.RemoveParticipant(ev.Participant.ID)
		} else {
			m.sidebar.UpdateParticipant(*ev.Participant)
		}
	case session.RoundEvent:
		m.round = ev.Round
		m.sidebar.SetRound(m.round)
		m.setStatus(hub.StatusIdle)
	case session.RoundStateEvent:
		m.round = ev.Round
		m.sidebar.SetRound(m.round)
		m.applyFlags(ev.IDs)
	}
}

func (m *ParticipantModel) forwardToChildren(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd

	if model, cmd := m.transcript.Update(msg); cmd != nil {
		m.transcript = model.(transcript.Model)
		cmds = append(cmds, cmd)
	}
	if model, cmd := m.sidebar.Update(msg); cmd != nil {
		m.sidebar = model.(sidebar.Model)
		cmds = append(cmds, cmd)
	}
	if model, cmd := m.response.Update(msg); cmd != nil {
		m.response = model.(response.Model)
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (m *ParticipantModel) forwardToFocused(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	switch m.activePane {
	case paneTranscript:
		if model, cmd := m.transcript.Update(msg); cmd != nil {
			m.transcript = model.(transcript.Model)
			cmds = append(cmds, cmd)
		}
	case paneSidebar:
		if model, cmd := m.sidebar.Update(msg); cmd != nil {
			m.sidebar = model.(sidebar.Model)
			cmds = append(cmds, cmd)
		}
	case paneResponse:
		if model, cmd := m.response.Update(msg); cmd != nil {
			m.response = model.(response.Model)
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (m *ParticipantModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "ctrl+c":
		return m, tea.Quit

	case "tab":
		if !m.namePromptActive {
			// Only cycle through transcript, sidebar, response (3 panes)
			m.activePane = (m.activePane + 1) % 3
			m.updateActiveStates()
		}
		return m, nil

	case "shift+tab":
		if !m.namePromptActive {
			// Only cycle through transcript, sidebar, response (3 panes)
			m.activePane = (m.activePane - 1 + 3) % 3
			m.updateActiveStates()
		}
		return m, nil
	}

	// Handle name prompt separately
	if m.namePromptActive {
		if keyStr == "enter" {
			name := strings.TrimSpace(m.namePrompt.Value())
			if name != "" {
				m.displayName = name
				m.hub.SetDisplayName(m.id, name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				m.sidebar.SetParticipants([]hub.Participant{
					{ID: m.id, DisplayName: name, Status: hub.StatusIdle},
				})
				return m, tea.Batch(
					m.transcript.Init(),
					m.sidebar.Init(),
					m.response.Init(),
					m.response.Focus(),
				)
			}
			return m, nil
		}

		var cmd tea.Cmd
		m.namePrompt, cmd = m.namePrompt.Update(msg)
		return m, cmd
	}

	switch m.activePane {
	case paneResponse:
		if keyStr == "enter" {
			return m.handleResponseSubmit()
		}
		fallthrough
	case paneTranscript, paneSidebar:
		cmds := m.forwardToFocused(msg)
		if m.activePane == paneResponse {
			m.syncTypingStatus()
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m *ParticipantModel) handleResponseSubmit() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.response.Value())
	if content == "" {
		return m, nil
	}

	msg := hub.NewMessage(hub.MessageTypeResponse, m.displayName, content, m.round)
	msg.FromID = m.id
	m.lastSubmitted = content
	m.lastSubmittedRound = msg.Round
	m.hub.Send(context.Background(), msg)
	m.setStatus(hub.StatusSubmitted)
	m.hub.SetDraft(m.id, "")

	m.response.Reset()
	return m, m.response.Focus()
}

// setStatus tells the hub what this participant is doing, once per change.
func (m *ParticipantModel) setStatus(status hub.ParticipantStatus) {
	if m.status == status {
		return
	}
	m.status = status
	m.hub.UpdateParticipantStatus(m.id, status)
}

// syncTypingStatus reports typing while the response box has text, and idle
// again if it is emptied. A submitted response stays submitted until the box
// has text again or the round changes.
func (m *ParticipantModel) syncTypingStatus() {
	m.hub.SetDraft(m.id, m.response.Value()) // so a reconnect can restore it
	switch {
	case strings.TrimSpace(m.response.Value()) != "":
		m.setStatus(hub.StatusTyping)
	case m.status == hub.StatusTyping:
		m.setStatus(hub.StatusIdle)
	}
}

func (m *ParticipantModel) updateActiveStates() {
	m.transcript.SetActive(m.activePane == paneTranscript)
	m.sidebar.SetActive(m.activePane == paneSidebar)
	m.response.SetActive(m.activePane == paneResponse)
}

func (m *ParticipantModel) layout() {
	leftWidth := m.width * 2 / 3
	rightWidth := m.width - leftWidth

	m.transcript.SetSize(leftWidth, m.height)
	m.sidebar.SetSize(rightWidth, m.height/2)
	m.response.SetSize(rightWidth, m.height/2)
}

func (m *ParticipantModel) View() string {
	if m.namePromptActive {
		return m.namePromptView()
	}

	m.layout()

	rightWidth := m.width - m.width*2/3
	rightTop := lipgloss.NewStyle().Width(rightWidth).Render(m.sidebar.View())
	rightBottom := lipgloss.NewStyle().Width(rightWidth).Render(m.response.View())

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightBottom)
	return lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)
}

func (m *ParticipantModel) namePromptView() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(1, 2)

	promptStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 2).
		Width(60)

	title := titleStyle.Render("Welcome to Haive Mind")
	prompt := promptStyle.Render("Enter display name:\n\n" + m.namePrompt.View())

	return lipgloss.JoinVertical(lipgloss.Center, title, prompt)
}

// Close ends the session's conversation with the hub. Call it when the session
// ends so the hub stops delivering to a model nobody is reading.
func (m *ParticipantModel) Close() {
	m.client.Close()
}

func (m *ParticipantModel) SetDisplayName(name string) {
	m.displayName = name
}

// applyFlags handles the coordinator's announcement of who the host has asked
// to revise. When this participant newly becomes flagged they are told, their
// last answer comes back for editing (unless they are already typing), and
// they are idle again.
func (m *ParticipantModel) applyFlags(ids []string) {
	m.sidebar.SetFlagged(ids)

	flagged := false
	for _, id := range ids {
		if id == m.id {
			flagged = true
		}
	}
	newlyFlagged := flagged && !m.flaggedSelf
	m.flaggedSelf = flagged
	if !newlyFlagged {
		return
	}
	m.transcript.AddSystemMessage("The host asked you to revise your response.")
	m.restoreLastSubmission()
	m.setStatus(hub.StatusIdle)
}

// restoreLastSubmission puts the last answer back for editing, unless the
// participant is already typing or the answer is from an earlier round.
func (m *ParticipantModel) restoreLastSubmission() {
	if strings.TrimSpace(m.response.Value()) == "" && m.lastSubmitted != "" && m.lastSubmittedRound == m.round {
		m.response.SetValue(m.lastSubmitted)
		m.hub.SetDraft(m.id, m.lastSubmitted)
	}
}
