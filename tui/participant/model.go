package participant

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/participant/response"
	"jakofranko/haive-mind/tui/participant/sidebar"
	"jakofranko/haive-mind/tui/participant/transcript"
)

const (
	paneTranscript = iota
	paneSidebar
	paneResponse
	paneNamePrompt
	numPanes
)

type ParticipantModel struct {
	hub         hub.Hub
	width       int
	height      int
	activePane  int
	round       int
	yourName    string
	sshUsername string

	transcript transcript.Model
	sidebar    sidebar.Model
	response   response.Model
	namePrompt textinput.Model

	namePromptActive bool
}

func NewParticipantModel(hubChans hub.Hub, yourName, sshUsername string) *ParticipantModel {
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
		yourName:         yourName,
		sshUsername:      sshUsername,
		transcript:       transcript.New(),
		sidebar:          sidebar.New(),
		response:         response.New(),
		namePrompt:       namePrompt,
		namePromptActive: false,
	}

	m.sidebar.SetYourName(yourName)
	m.sidebar.SetParticipants([]hub.Participant{
		{Name: yourName, DisplayName: yourName, Status: hub.StatusIdle, Round: 0},
	})

	// Check if we have a stored display name
	if displayName := hubChans.GetDisplayName(sshUsername); displayName != "" {
		m.yourName = displayName
		m.sidebar.SetYourName(displayName)
		m.sidebar.SetParticipants([]hub.Participant{
			{Name: displayName, DisplayName: displayName, Status: hub.StatusIdle, Round: 0},
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
		return textinput.Blink
	}
	return tea.Batch(
		m.transcript.Init(),
		m.sidebar.Init(),
		m.response.Init(),
		m.listenForMessages(),
		m.listenForParticipantUpdates(),
		m.listenForRoundChanges(),
		m.listenForErrors(),
	)
}

type hubMessageMsg hub.Message
type participantUpdateMsg hub.Participant
type roundChangedMsg int
type errorMsg error

func (m *ParticipantModel) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.hub.Inbound()
		return hubMessageMsg(*msg)
	}
}

func (m *ParticipantModel) listenForParticipantUpdates() tea.Cmd {
	return func() tea.Msg {
		p := <-m.hub.ParticipantUpdateChan()
		return participantUpdateMsg(p)
	}
}

func (m *ParticipantModel) listenForRoundChanges() tea.Cmd {
	return func() tea.Msg {
		round := <-m.hub.RoundChangedChan()
		return roundChangedMsg(round)
	}
}

func (m *ParticipantModel) listenForErrors() tea.Cmd {
	return func() tea.Msg {
		err := <-m.hub.ErrorChan()
		return errorMsg(err)
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

	case hubMessageMsg:
		m.transcript.AddMessage(hub.Message(msg))
		cmds = append(cmds, m.listenForMessages())
		return m, tea.Batch(cmds...)

	case hub.Message:
		m.transcript.AddMessage(msg)
		cmds = append(cmds, m.listenForMessages())
		return m, tea.Batch(cmds...)

	case participantUpdateMsg:
		m.sidebar.UpdateParticipant(hub.Participant(msg))
		cmds = append(cmds, m.listenForParticipantUpdates())
		return m, tea.Batch(cmds...)

	case roundChangedMsg:
		m.round = int(msg)
		m.sidebar.SetRound(m.round)
		cmds = append(cmds, m.listenForRoundChanges())
		return m, tea.Batch(cmds...)

	case errorMsg:
		m.transcript.AddSystemMessage("Error: " + msg.Error())
		cmds = append(cmds, m.listenForErrors())
		return m, tea.Batch(cmds...)
	}

	cmds = append(cmds, m.forwardToFocused(msg)...)
	return m, tea.Batch(cmds...)
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
				m.yourName = name
				m.hub.SetDisplayName(m.sshUsername, name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				m.sidebar.SetYourName(name)
				m.sidebar.SetParticipants([]hub.Participant{
					{Name: name, DisplayName: name, Status: hub.StatusIdle, Round: 0},
				})
				// Send participant update to hub so host panel shows this participant
				m.hub.ParticipantUpdateChan() <- hub.Participant{
					Name:        name,
					DisplayName: name,
					Status:      hub.StatusIdle,
					Round:       m.round,
				}
				return m, tea.Batch(
					m.transcript.Init(),
					m.sidebar.Init(),
					m.response.Init(),
					m.listenForMessages(),
					m.listenForParticipantUpdates(),
					m.listenForRoundChanges(),
					m.listenForErrors(),
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
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m *ParticipantModel) handleResponseSubmit() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.response.Value())
	if content == "" {
		return m, nil
	}

	msg := hub.NewMessage(hub.MessageTypeResponse, m.yourName, content, m.round)
	m.hub.SendChan() <- msg

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

func (m *ParticipantModel) SetYourName(name string) {
	m.yourName = name
	m.sidebar.SetYourName(name)
}
