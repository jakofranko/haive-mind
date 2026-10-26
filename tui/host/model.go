package host

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
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
	paneNamePrompt
	numPanes
)

// AppModel is the main TUI model that composes child pane models
type AppModel struct {
	hub        hub.Hub
	width      int
	height     int
	activePane int
	round      int
	yourName   string
	sshUsername string

	// Child models
	transcript   transcript.Model
	participants participants.Model
	directive    directive.Model
	buttons      buttons.Model
	toast        toast.Model

	// Name prompt
	namePrompt     textinput.Model
	namePromptActive bool
}

func NewAppModel(hubChans hub.Hub) *AppModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &AppModel{
		hub:            hubChans,
		activePane:     paneTranscript,
		round:          1,
		yourName:       "Host",
		sshUsername:    "host",
		transcript:     transcript.New(),
		participants:   participants.New(),
		directive:      directive.New(),
		buttons:        buttons.New(),
		toast:          toast.New(),
		namePrompt:     namePrompt,
		namePromptActive: false,
	}

	// Check if we have a stored display name
	if displayName := hubChans.GetDisplayName("host"); displayName != "" {
		m.yourName = displayName
	} else {
		// First time - activate name prompt
		m.activePane = paneNamePrompt
		m.namePromptActive = true
	}

	return m
}

func (m *AppModel) Init() tea.Cmd {
	if m.namePromptActive {
		return textinput.Blink
	}
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
		cmds = append(cmds, m.forwardToChildren(msg)...)
		updatedToast, cmd := m.toast.Update(msg)
		m.toast = updatedToast.(toast.Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
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
		p := hub.Participant(msg)
		if p.Status == hub.StatusDisconnected {
			m.participants.RemoveParticipant(p.Name)
		} else {
			m.participants.UpdateParticipant(p)
		}
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
	}

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

	switch keyStr {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		if !m.namePromptActive {
			m.activePane = (m.activePane + 1) % (numPanes - 1) // Exclude namePrompt pane
			m.updateActiveStates()
		}
		return m, nil

	case "shift+tab":
		if !m.namePromptActive {
			m.activePane = (m.activePane - 1 + (numPanes - 1)) % (numPanes - 1)
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
				m.hub.SetDisplayName("host", name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				// Send participant update so host panel refreshes
				m.hub.ParticipantUpdateChan() <- hub.Participant{
					Name:        "host",
					DisplayName: name,
					Status:      hub.StatusIdle,
					Round:       m.round,
				}
				return m, tea.Batch(
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
					m.directive.Focus(),
				)
			}
			return m, nil
		}

		var cmd tea.Cmd
		m.namePrompt, cmd = m.namePrompt.Update(msg)
		return m, cmd
	}

	switch m.activePane {
	case paneDirective:
		if keyStr == "enter" && !isCtrlPressed(msg) {
			return m.handleDirectiveSend()
		}
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
		if model, cmd := m.buttons.Update(msg); true {
			m.buttons = model.(buttons.Model)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case paneTranscript:
		if model, cmd := m.transcript.Update(msg); true {
			m.transcript = model.(transcript.Model)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case paneParticipants:
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

	msg := hub.NewMessage(hub.MessageTypeDirective, m.yourName, content, m.round)
	m.hub.Outbound() <- msg
	m.hub.SendChan() <- msg
	m.transcript.AddMessage(*msg)

	m.directive.Reset()
	return m, m.directive.Focus()
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

	m.transcript.SetSize(leftWidth, m.height)

	rightPaneHeight := (m.height - 4) / 3
	if rightPaneHeight < 4 {
		rightPaneHeight = 4
	}

	m.participants.SetSize(rightWidth, rightPaneHeight)
	m.directive.SetSize(rightWidth, rightPaneHeight)
	m.buttons.SetSize(rightWidth, rightPaneHeight)
}

func (m *AppModel) View() string {
	if m.namePromptActive {
		return m.namePromptView()
	}

	m.layout()

	rightWidth := m.width - m.width/2
	rightTop := lipgloss.NewStyle().Width(rightWidth).Render(m.participants.View())
	rightMid := lipgloss.NewStyle().Width(rightWidth).Render(m.directive.View())
	rightBottom := lipgloss.NewStyle().Width(rightWidth).Render(m.buttons.View())

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightMid, rightBottom)
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, m.transcript.View(), rightPane)

	toastView := m.toast.View()
	if toastView != "" {
		return lipgloss.JoinVertical(lipgloss.Left, mainView, toastView)
	}
	return mainView
}

func (m *AppModel) namePromptView() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(1, 2)

	promptStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 2).
		Width(60)

	title := titleStyle.Render("Welcome to Haive Mind (Host)")
	prompt := promptStyle.Render("Enter display name:\n\n" + m.namePrompt.View())

	return lipgloss.JoinVertical(lipgloss.Center, title, prompt)
}