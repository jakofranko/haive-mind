package host

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/host/buttons"
	"jakofranko/haive-mind/tui/host/directive"
	"jakofranko/haive-mind/tui/host/participants"
	"jakofranko/haive-mind/tui/host/toast"
	"jakofranko/haive-mind/tui/host/transcript"
)

const (
	paneTranscript = iota
	paneParticipants
	paneDirective
	paneButtons
	numPanes
)

// AppModel is the main TUI model that composes child pane models
type AppModel struct {
	hub        hub.Hub
	width      int
	height     int
	activePane int
	round      int

	// Child models
	transcript   transcript.Model
	participants participants.Model
	directive    directive.Model
	buttons      buttons.Model
	toast        toast.Model

	// Agent loop state
	pendingDirective string
	pendingAgents    []string
	currentAgentIdx  int
}

func NewAppModel(hubChans hub.Hub) *AppModel {
	m := &AppModel{
		hub:           hubChans,
		activePane:    paneTranscript,
		round:         1,
		transcript:    transcript.New(),
		participants:  participants.New(),
		directive:     directive.New(),
		buttons:       buttons.New(),
		toast:         toast.New(),
		pendingAgents: []string{"Agent-1", "Agent-2", "Agent-3"},
	}

	// Set initial participants
	m.participants.SetParticipants([]hub.Participant{
		{Name: "Agent-1", Status: hub.StatusIdle, Round: 0},
		{Name: "Agent-2", Status: hub.StatusIdle, Round: 0},
		{Name: "Agent-3", Status: hub.StatusIdle, Round: 0},
	})

	return m
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.transcript.Init(),
		m.participants.Init(),
		m.directive.Init(),
		m.buttons.Init(),
		m.toast.Init(),
		m.listenForMessages(),
		m.listenForParticipantUpdates(),
		m.listenForRoundChanges(),
		m.listenForExportComplete(),
		m.listenForErrors(),
	)
}

type hubMessageMsg hub.Message
type participantUpdateMsg hub.Participant
type roundChangedMsg int
type exportCompleteMsg string
type errorMsg error
type runNextAgentMsg string
type agentResponseMsg struct {
	agentName string
	response  string
	err       error
}

func (m *AppModel) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.hub.Inbound()
		return hubMessageMsg(*msg)
	}
}

func (m *AppModel) listenForParticipantUpdates() tea.Cmd {
	return func() tea.Msg {
		p := <-m.hub.ParticipantUpdateChan()
		return participantUpdateMsg(p)
	}
}

func (m *AppModel) listenForRoundChanges() tea.Cmd {
	return func() tea.Msg {
		round := <-m.hub.RoundChangedChan()
		return roundChangedMsg(round)
	}
}

func (m *AppModel) listenForExportComplete() tea.Cmd {
	return func() tea.Msg {
		path := <-m.hub.ExportCompleteChan()
		return exportCompleteMsg(path)
	}
}

func (m *AppModel) listenForErrors() tea.Cmd {
	return func() tea.Msg {
		err := <-m.hub.ErrorChan()
		return errorMsg(err)
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		// Forward to all children
		cmds = append(cmds, m.forwardToChildren(msg)...)
		updatedToast, cmd := m.toast.Update(msg)
		m.toast = updatedToast.(toast.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// Let toast handle keys first if visible
		if m.toast.IsVisible() {
			updatedToast, cmd := m.toast.Update(msg)
			m.toast = updatedToast.(toast.Model)
			return m, cmd
		}
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
		m.participants.UpdateParticipant(hub.Participant(msg))
		cmds = append(cmds, m.listenForParticipantUpdates())
		return m, tea.Batch(cmds...)

	case roundChangedMsg:
		m.round = int(msg)
		m.participants.SetRound(m.round)
		cmds = append(cmds, m.listenForRoundChanges())
		return m, tea.Batch(cmds...)

	case exportCompleteMsg:
		m.transcript.AddSystemMessage("Exported to " + string(msg))
		cmds = append(cmds, m.listenForExportComplete())
		return m, tea.Batch(cmds...)

	case errorMsg:
		m.transcript.AddSystemMessage("Error: " + msg.Error())
		cmds = append(cmds, m.listenForErrors())
		return m, tea.Batch(cmds...)

	case runNextAgentMsg:
		return m.runNextAgent(string(msg))

	case agentResponseMsg:
		return m.handleAgentResponse(msg)
	}

	// Forward unhandled messages to focused child
	cmds = append(cmds, m.forwardToFocused(msg)...)
	return m, tea.Batch(cmds...)
}

func (m *AppModel) forwardToChildren(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	if model, cmd := m.transcript.Update(msg); cmd != nil || true {
		m.transcript = model.(transcript.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if model, cmd := m.participants.Update(msg); cmd != nil || true {
		m.participants = model.(participants.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if model, cmd := m.directive.Update(msg); cmd != nil || true {
		m.directive = model.(directive.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if model, cmd := m.buttons.Update(msg); cmd != nil || true {
		m.buttons = model.(buttons.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if model, cmd := m.toast.Update(msg); cmd != nil || true {
		m.toast = model.(toast.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (m *AppModel) forwardToFocused(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	switch m.activePane {
	case paneTranscript:
		if model, cmd := m.transcript.Update(msg); cmd != nil || true {
			m.transcript = model.(transcript.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case paneDirective:
		if model, cmd := m.directive.Update(msg); cmd != nil || true {
			m.directive = model.(directive.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case paneParticipants:
		if model, cmd := m.participants.Update(msg); cmd != nil || true {
			m.participants = model.(participants.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case paneButtons:
		if model, cmd := m.buttons.Update(msg); cmd != nil || true {
			m.buttons = model.(buttons.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

func (m *AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Global pane navigation
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

	// Pane-specific handling
	switch m.activePane {
	case paneDirective:
		// Intercept Enter (without Ctrl) to send directive
		if keyStr == "enter" && !isCtrlPressed(msg) {
			return m.handleDirectiveSend()
		}
		// Forward to directive model - always update child state
		if model, cmd := m.directive.Update(msg); true {
			m.directive = model.(directive.Model)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case paneButtons:
		if keyStr == "enter" {
			return m.handleButtonPress()
		}
		// Forward left/right to buttons model - always update child state
		if model, cmd := m.buttons.Update(msg); true {
			m.buttons = model.(buttons.Model)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case paneTranscript:
		// Forward to transcript model - always update child state
		if model, cmd := m.transcript.Update(msg); true {
			m.transcript = model.(transcript.Model)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case paneParticipants:
		// Participants pane is read-only for now
		return m, nil
	}

	return m, nil
}

func (m *AppModel) updateActiveStates() {
	m.transcript.SetActive(m.activePane == paneTranscript)
	m.participants.SetActive(m.activePane == paneParticipants)
	m.directive.SetActive(m.activePane == paneDirective)
	m.buttons.SetActive(m.activePane == paneButtons)
}

func isCtrlPressed(msg tea.KeyMsg) bool {
	return strings.HasPrefix(msg.String(), "ctrl+")
}

func (m *AppModel) handleDirectiveSend() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.directive.Value())
	if content == "" {
		return m, nil
	}

	msg := hub.NewMessage(hub.MessageTypeDirective, "host", content, m.round)
	m.hub.Outbound() <- msg // For local transcript display
	m.hub.SendChan() <- msg // For hub to send to agents
	m.transcript.AddMessage(*msg)

	m.directive.Reset()

	// Start agent loop
	m.pendingDirective = content
	m.currentAgentIdx = 0
	return m, func() tea.Msg {
		return runNextAgentMsg(m.pendingAgents[0])
	}
}

func (m *AppModel) runNextAgent(agentName string) (tea.Model, tea.Cmd) {
	if m.currentAgentIdx >= len(m.pendingAgents) {
		// All agents done, advance round
		m.round++
		m.hub.NewRoundChan() <- m.round
		m.pendingDirective = ""
		return m, nil
	}

	agent := m.pendingAgents[m.currentAgentIdx]
	// Update participant status to typing
	m.hub.ParticipantUpdateChan() <- hub.Participant{Name: agent, Status: hub.StatusTyping, Round: m.round}

	return m, func() tea.Msg {
		ctx := context.Background()
		response, err := m.hub.SendToAgent(ctx, m.pendingDirective)
		return agentResponseMsg{agentName: agent, response: response, err: err}
	}
}

func (m *AppModel) handleAgentResponse(msg agentResponseMsg) (tea.Model, tea.Cmd) {
	agent := msg.agentName

	if msg.err != nil {
		// Show toast with retry
		m.hub.ParticipantUpdateChan() <- hub.Participant{Name: agent, Status: hub.StatusIdle, Round: m.round}
		return m, toast.ShowToast(
			"Agent "+agent+" failed: "+msg.err.Error(),
			true,
			func() tea.Cmd {
				return func() tea.Msg {
					return runNextAgentMsg(agent)
				}
			},
		)
	}

	// Success - add response to transcript and broadcast
	responseMsg := hub.NewMessage(hub.MessageTypeResponse, agent, msg.response, m.round)
	m.hub.Outbound() <- responseMsg
	m.transcript.AddMessage(*responseMsg)

	// Update participant status
	m.hub.ParticipantUpdateChan() <- hub.Participant{Name: agent, Status: hub.StatusReady, Round: m.round}

	// Move to next agent
	m.currentAgentIdx++
	if m.currentAgentIdx < len(m.pendingAgents) {
		nextAgent := m.pendingAgents[m.currentAgentIdx]
		return m, func() tea.Msg {
			return runNextAgentMsg(nextAgent)
		}
	}

	// All agents done, advance round
	m.round++
	m.hub.NewRoundChan() <- m.round
	m.pendingDirective = ""
	return m, nil
}

func (m *AppModel) handleButtonPress() (tea.Model, tea.Cmd) {
	activeBtn := m.buttons.ActiveButton()
	switch activeBtn {
	case "[Aggregate]":
		m.hub.AggregateChan() <- m.round
	case "[Send]":
		return m.handleDirectiveSend()
	case "[New Round]":
		m.round++
		m.hub.NewRoundChan() <- m.round
	case "[Export]":
		exportPath := "transcript_" + time.Now().Format("20060102_150405") + ".json"
		m.hub.ExportChan() <- exportPath
	}
	return m, nil
}

func (m *AppModel) layout() {
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth

	// Left: transcript full height
	m.transcript.SetSize(leftWidth, m.height)

	// Right: 3 equal panes
	rightPaneHeight := (m.height - 4) / 3
	if rightPaneHeight < 4 {
		rightPaneHeight = 4
	}

	m.participants.SetSize(rightWidth, rightPaneHeight)
	m.directive.SetSize(rightWidth, rightPaneHeight)
	m.buttons.SetSize(rightWidth, rightPaneHeight)
}

func (m *AppModel) View() string {
	m.layout()

	rightWidth := m.width - m.width/2
	rightTop := lipgloss.NewStyle().Width(rightWidth).Render(m.participants.View())
	rightMid := lipgloss.NewStyle().Width(rightWidth).Render(m.directive.View())
	rightBottom := lipgloss.NewStyle().Width(rightWidth).Render(m.buttons.View())

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightMid, rightBottom)
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)

	// Add toast overlay
	toastView := m.toast.View()
	if toastView != "" {
		return lipgloss.JoinVertical(lipgloss.Left, mainView, toastView)
	}
	return mainView
}
