package participant

import (
	"strings"

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
	numPanes
)

type ParticipantModel struct {
	hub        *hub.HubChannels
	width      int
	height     int
	activePane int
	round      int
	yourName   string

	transcript transcript.Model
	sidebar    sidebar.Model
	response   response.Model
}

func NewParticipantModel(hubChans *hub.HubChannels, yourName string) *ParticipantModel {
	m := &ParticipantModel{
		hub:        hubChans,
		activePane: paneTranscript,
		round:      1,
		yourName:   yourName,

		transcript: transcript.New(),
		sidebar:    sidebar.New(),
		response:   response.New(),
	}

	m.sidebar.SetYourName(yourName)
	m.sidebar.SetParticipants([]hub.Participant{
		{Name: yourName, Status: hub.StatusIdle, Round: 0},
	})

	return m
}

func (m *ParticipantModel) Init() tea.Cmd {
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
		msg := <-m.hub.Inbound
		return hubMessageMsg(*msg)
	}
}

func (m *ParticipantModel) listenForParticipantUpdates() tea.Cmd {
	return func() tea.Msg {
		p := <-m.hub.ParticipantUpdateChan
		return participantUpdateMsg(p)
	}
}

func (m *ParticipantModel) listenForRoundChanges() tea.Cmd {
	return func() tea.Msg {
		round := <-m.hub.RoundChangedChan
		return roundChangedMsg(round)
	}
}

func (m *ParticipantModel) listenForErrors() tea.Cmd {
	return func() tea.Msg {
		err := <-m.hub.ErrorChan
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
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		m.activePane = (m.activePane + 1) % numPanes
		m.updateActiveStates()
		return m, nil

	case "shift+tab":
		m.activePane = (m.activePane - 1 + numPanes) % numPanes
		m.updateActiveStates()
		return m, nil
	}

	switch m.activePane {
	case paneResponse:
		if keyStr == "ctrl+enter" {
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
	m.hub.Outbound <- msg
	m.hub.SendChan <- msg
	m.transcript.AddMessage(*msg)

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
	m.layout()

	rightWidth := m.width - m.width*2/3
	rightTop := lipgloss.NewStyle().Width(rightWidth).Render(m.sidebar.View())
	rightBottom := lipgloss.NewStyle().Width(rightWidth).Render(m.response.View())

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightBottom)
	return lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)
}

func (m *ParticipantModel) SetYourName(name string) {
	m.yourName = name
	m.sidebar.SetYourName(name)
}