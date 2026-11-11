package host

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/host/buttons"
	"jakofranko/haive-mind/tui/host/directive"
	"jakofranko/haive-mind/tui/host/participants"
	"jakofranko/haive-mind/tui/host/toast"
	"jakofranko/haive-mind/tui/session"
	"jakofranko/haive-mind/tui/transcript"
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
	width      int
	height     int
	activePane int

	// Child models
	transcript   transcript.Model
	participants participants.Model
	directive    directive.Model
	buttons      buttons.Model
	toast        toast.Model

	// Name prompt
	namePrompt       textinput.Model
	namePromptActive bool

	// draftingAggregate is true while the directive input holds a synthesized
	// aggregate for the host to edit; Enter then sends it as the aggregate.
	draftingAggregate bool

	// The session's conversation with the hub: it knows the round and sends
	// on the host's behalf.
	client *session.Host
}

func NewAppModel(hubChans hub.Hub) *AppModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &AppModel{
		activePane:   paneTranscript,
		transcript:   transcript.New("Welcome to Haive Mind Host TUI\nPress Tab to navigate panes"),
		participants: participants.New(),
		directive:    directive.New(),
		buttons:      buttons.New(),
		toast:        toast.New(),
		namePrompt:   namePrompt,
		client:       session.NewHost(hubChans),
	}

	if m.client.DisplayName() == "" {
		// First time - activate name prompt
		m.activePane = paneNamePrompt
		m.namePromptActive = true
	}

	return m
}

// Close ends the session's conversation with the hub. Call it when the session
// ends so the hub stops delivering to a model nobody is reading.
func (m *AppModel) Close() {
	m.client.Close()
}

func (m *AppModel) Init() tea.Cmd {
	if m.namePromptActive {
		return tea.Batch(textinput.Blink, m.waitForEvent())
	}
	return tea.Batch(
		m.transcript.Init(),
		m.participants.Init(),
		m.directive.Init(),
		m.buttons.Init(),
		m.toast.Init(),
		m.waitForEvent(),
	)
}

// eventMsg carries one event from the session client into Update.
type eventMsg struct{ session.Event }

// errorMsg is a failed retry, returned by the retry path to the model.
type errorMsg error

// waitForEvent pops the client's next event; Update re-arms it. It returns
// nil once the client is closed, which ends the wait.
func (m *AppModel) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := m.client.Next()
		if !ok {
			return nil
		}
		return eventMsg{ev}
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout() // sizes every pane; they need no WindowSizeMsg of their own
		return m, nil

	case tea.KeyMsg:
		if m.toast.IsVisible() {
			updatedToast, cmd := m.toast.Update(msg)
			m.toast = updatedToast.(toast.Model)
			return m, cmd
		}
		return m.handleKeyMsg(msg)

	case eventMsg:
		return m, tea.Batch(m.reduce(msg.Event), m.waitForEvent())

	case errorMsg:
		m.transcript.AddSystemMessage("Error: " + msg.Error())
		return m, nil
	}

	updatedToast, toastCmd := m.toast.Update(msg)
	m.toast = updatedToast.(toast.Model)
	return m, tea.Batch(toastCmd, m.forwardToFocused(msg))
}

// reduce applies one event from the session client to the panes, and returns
// a command when the event calls for one (the agent-failure toast).
func (m *AppModel) reduce(ev session.Event) tea.Cmd {
	switch ev := ev.(type) {
	case session.CatchUpEvent:
		m.transcript.PrependMessages(ev.Messages)
		m.participants.SetParticipants(ev.Participants)
		m.participants.SetRound(ev.Round)
		m.participants.SetFlagged(ev.Flagged)
	case session.MessageEvent:
		m.transcript.AddMessage(ev.Message)
	case session.DraftEvent:
		m.transcript.AddMessage(ev.Message)
		m.openDraft(ev.Message.Content)
	case session.AgentFailedEvent:
		return m.showAgentError(ev)
	case session.ParticipantEvent:
		if ev.Participant.Status == hub.StatusDisconnected {
			m.participants.RemoveParticipant(ev.Participant.ID)
		} else {
			m.participants.UpdateParticipant(*ev.Participant)
		}
	case session.RoundEvent:
		m.participants.SetRound(ev.Round)
		m.participants.SetFlagged(ev.Flagged)
	case session.ExportEvent:
		m.transcript.AddSystemMessage("Exported to " + ev.Path)
	}
	return nil
}

// forwardToFocused hands msg to the focused pane and keeps its updated model,
// whether or not it returns a command: a viewport scrolls without one.
func (m *AppModel) forwardToFocused(msg tea.Msg) tea.Cmd {
	var model tea.Model
	var cmd tea.Cmd
	switch m.activePane {
	case paneTranscript:
		model, cmd = m.transcript.Update(msg)
		m.transcript = model.(transcript.Model)
	case paneDirective:
		model, cmd = m.directive.Update(msg)
		m.directive = model.(directive.Model)
	case paneParticipants:
		model, cmd = m.participants.Update(msg)
		m.participants = model.(participants.Model)
	case paneButtons:
		model, cmd = m.buttons.Update(msg)
		m.buttons = model.(buttons.Model)
	}
	return cmd
}

func (m *AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	switch keyStr {
	case "ctrl+c":
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
				m.client.Rename(name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				return m, tea.Batch(
					m.transcript.Init(),
					m.participants.Init(),
					m.directive.Init(),
					m.buttons.Init(),
					m.toast.Init(),
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
		if keyStr == "esc" && m.draftingAggregate {
			m.draftingAggregate = false
			m.directive.Reset()
			return m, nil
		}
		if keyStr == "enter" && !isCtrlPressed(msg) {
			return m.handleDirectiveSend()
		}
	case paneButtons:
		if keyStr == "enter" {
			return m.handleButtonPress()
		}
	case paneParticipants:
		if keyStr == "c" {
			return m.toggleFlag()
		}
	}
	return m, m.forwardToFocused(msg)
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
	send := m.client.SendDirective
	if m.draftingAggregate {
		send = m.client.SendAggregate
	}
	if !send(m.directive.Value()) {
		return m, nil
	}
	m.draftingAggregate = false

	m.directive.Reset()
	return m, m.directive.Focus()
}

// openDraft puts a synthesized aggregate in the input for the host to edit.
func (m *AppModel) openDraft(content string) {
	m.draftingAggregate = true
	m.directive.SetValue(content)
	m.activePane = paneDirective
	m.updateActiveStates()
}

func (m *AppModel) handleButtonPress() (tea.Model, tea.Cmd) {
	activeBtn := m.buttons.ActiveButton()
	switch activeBtn {
	case "[Aggregate]":
		if n := m.participants.FlaggedCount(); n > 0 {
			return m, m.confirmAggregate(n)
		}
		m.client.RequestAggregate()
	case "[Send]":
		return m.handleDirectiveSend()
	case "[New Round]":
		// The coordinator owns the next number; the client names the round
		// being ended, so a repeat is ignored.
		m.client.NewRound()
	case "[Resolve]":
		m.client.Resolve()
	case "[Export]":
		m.client.Export()
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

// showAgentError raises a toast for a failed agent call; Retry sends the failed
// message again.
func (m *AppModel) showAgentError(failed session.AgentFailedEvent) tea.Cmd {
	var retry func() tea.Cmd
	if failed.CanRetry {
		retry = func() tea.Cmd {
			return func() tea.Msg {
				if err := m.client.Retry(failed); err != nil {
					return errorMsg(err)
				}
				return nil
			}
		}
	}
	return toast.ShowToast(failed.Text, true, retry)
}

// toggleFlag asks the coordinator to flag (or unflag) the selected
// participant. The client refuses to flag the host.
func (m *AppModel) toggleFlag() (tea.Model, tea.Cmd) {
	p, ok := m.participants.Selected()
	if !ok {
		return m, nil
	}
	if m.participants.IsFlagged(p.ID) {
		m.client.Unflag(p.ID)
	} else {
		m.client.Flag(p.ID)
	}
	return m, nil
}

// confirmAggregate warns that participants are still flagged and offers to
// aggregate anyway.
func (m *AppModel) confirmAggregate(flagged int) tea.Cmd {
	who := "participant is"
	if flagged != 1 {
		who = "participants are"
	}
	text := fmt.Sprintf("%d %s still flagged for revision. Aggregate anyway?", flagged, who)
	return toast.ShowAction(text, "Continue", func() tea.Cmd {
		return func() tea.Msg {
			m.client.RequestAggregate()
			return nil
		}
	})
}
