package participant

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/participant/response"
	"jakofranko/haive-mind/tui/participant/sidebar"
	"jakofranko/haive-mind/tui/session"
	"jakofranko/haive-mind/tui/transcript"
)

const (
	paneTranscript = iota
	paneSidebar
	paneResponse
	paneNamePrompt
	numPanes
)

type ParticipantModel struct {
	width      int
	height     int
	activePane int
	id         string // stable hub identity (set by the SSH server); never changes

	transcript transcript.Model
	sidebar    sidebar.Model
	response   response.Model
	namePrompt textinput.Model

	namePromptActive bool

	// The session's conversation with the hub: it knows where this
	// participant stands in the round and sends on their behalf.
	client *session.Participant
}

func NewParticipantModel(hubChans hub.Hub, id, sshUsername string) *ParticipantModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &ParticipantModel{
		activePane: paneTranscript,
		id:         id,
		transcript: transcript.New("Welcome to Haive Mind Participant TUI\nWaiting for messages..."),
		sidebar:    sidebar.New(id),
		response:   response.New(),
		namePrompt: namePrompt,
		client:     session.NewParticipant(hubChans, id),
	}

	// Pick up text a previous session left unsent.
	if draft := m.client.UnsentText(); draft != "" {
		m.response.SetDraft(draft)
	}

	// Until Catch-up brings the roster, the sidebar shows this participant.
	displayName := m.client.DisplayName()
	if displayName == "" {
		displayName = id
		m.activePane = paneNamePrompt
		m.namePromptActive = true
	}
	m.sidebar.SetParticipants([]hub.Participant{
		{ID: m.id, DisplayName: displayName, Status: hub.StatusIdle},
	})

	return m
}

func (m *ParticipantModel) Init() tea.Cmd {
	if m.namePromptActive {
		return tea.Batch(textinput.Blink, m.waitForEvent())
	}
	return tea.Batch(
		m.transcript.Init(),
		m.sidebar.Init(),
		m.response.Init(),
		m.waitForEvent(),
	)
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
	}

	cmds = append(cmds, m.forwardToFocused(msg)...)
	return m, tea.Batch(cmds...)
}

// reduce applies one event from the session client to the panes.
func (m *ParticipantModel) reduce(ev session.Event) {
	switch ev := ev.(type) {
	case session.CatchUpEvent:
		m.transcript.PrependMessages(ev.Messages)
		m.sidebar.SetParticipants(ev.Participants)
		m.sidebar.SetRound(ev.Round)
		m.sidebar.SetFlagged(ev.Flagged)
	case session.MessageEvent:
		m.transcript.AddMessage(ev.Message)
	case session.ParticipantEvent:
		if ev.Participant.Status == hub.StatusDisconnected {
			m.sidebar.RemoveParticipant(ev.Participant.ID)
		} else {
			m.sidebar.UpdateParticipant(*ev.Participant)
		}
	case session.RoundEvent:
		m.sidebar.SetRound(ev.Round)
		m.sidebar.SetFlagged(ev.Flagged)
	case session.RevisionEvent:
		m.transcript.AddSystemMessage("The host asked you to revise your response.")
		// Their last answer comes back to edit, unless they are already typing.
		if ev.LastAnswer != "" && strings.TrimSpace(m.response.Value()) == "" {
			m.response.SetValue(ev.LastAnswer)
		}
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
				m.client.Rename(name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				m.sidebar.UpdateParticipant(hub.Participant{ID: m.id, DisplayName: name, Status: hub.StatusIdle})
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
			m.client.Typed(m.response.Value())
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m *ParticipantModel) handleResponseSubmit() (tea.Model, tea.Cmd) {
	if !m.client.Submit(m.response.Value()) {
		return m, nil
	}
	m.response.Reset()
	return m, m.response.Focus()
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
