package host

import (
	"context"
	"fmt"
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
	"jakofranko/haive-mind/tui/session"
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
	hub         hub.Hub
	width       int
	height      int
	activePane  int
	round       int
	displayName string

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

	// The session's conversation with the hub.
	client *session.Client
}

func NewAppModel(hubChans hub.Hub) *AppModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &AppModel{
		hub:              hubChans,
		activePane:       paneTranscript,
		round:            1,
		displayName:      "Host",
		transcript:       transcript.New(),
		participants:     participants.New(),
		directive:        directive.New(),
		buttons:          buttons.New(),
		toast:            toast.New(),
		namePrompt:       namePrompt,
		namePromptActive: false,
		// The host also hears about failed agent calls and finished exports.
		client: session.New(hubChans, hub.MessageTypeAgentError, hub.MessageTypeExportComplete),
	}

	// Check if we have a stored display name
	if displayName := hubChans.GetDisplayName(hub.HostID); displayName != "" {
		m.displayName = displayName
	} else {
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
		return tea.Batch(textinput.Blink, m.waitForEvent(), m.initialParticipants())
	}
	return tea.Batch(
		m.transcript.Init(),
		m.participants.Init(),
		m.directive.Init(),
		m.buttons.Init(),
		m.toast.Init(),
		m.waitForEvent(),
		m.initialParticipants(),
	)
}

type initialParticipantsMsg []hub.Participant

func (m *AppModel) initialParticipants() tea.Cmd {
	return func() tea.Msg { return initialParticipantsMsg(m.client.Participants()) }
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

	case eventMsg:
		return m, tea.Batch(m.reduce(msg.Event), m.waitForEvent())

	case initialParticipantsMsg:
		m.participants.SetParticipants(msg)
		return m, nil

	case errorMsg:
		m.transcript.AddSystemMessage("Error: " + msg.Error())
		return m, nil
	}

	if updated, cmd := m.toast.Update(msg); true {
		m.toast = updated.(toast.Model)
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.forwardToFocused(msg)...)
	return m, tea.Batch(cmds...)
}

// reduce applies one event from the session client to the panes, and returns
// a command when the event calls for one (the agent-failure toast).
func (m *AppModel) reduce(ev session.Event) tea.Cmd {
	switch ev := ev.(type) {
	case session.MessageEvent:
		switch ev.Message.Type {
		case hub.MessageTypeAgentError:
			return m.showAgentError(ev.Message)
		case hub.MessageTypeAggregateDraft:
			m.transcript.AddMessage(ev.Message)
			m.openDraft(ev.Message.Content)
		default:
			m.transcript.AddMessage(ev.Message)
		}
	case session.ParticipantEvent:
		if ev.Participant.Status == hub.StatusDisconnected {
			m.participants.RemoveParticipant(ev.Participant.ID)
		} else {
			m.participants.UpdateParticipant(*ev.Participant)
		}
	case session.RoundEvent:
		m.round = ev.Round
		m.participants.SetRound(m.round)
	case session.RoundStateEvent:
		m.round = ev.Round
		m.participants.SetRound(m.round)
		m.participants.SetFlagged(ev.IDs)
	case session.ExportEvent:
		m.transcript.AddSystemMessage("Exported to " + ev.Path)
	}
	return nil
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
				m.displayName = name
				m.hub.SetDisplayName(hub.HostID, name)
				m.namePromptActive = false
				m.activePane = paneTranscript
				m.namePrompt.Blur()
				// Send participant update so host panel refreshes
				p := hub.NewParticipantEvent(hub.HostID, name, hub.StatusIdle, m.round, hub.ParticipantEventStatusChange)
				m.hub.RegisterParticipant(p)
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
		if keyStr == "c" {
			return m.toggleFlag()
		}
		if model, _ := m.participants.Update(msg); true {
			m.participants = model.(participants.Model)
		}
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

	msgType := hub.MessageTypeDirective
	if m.draftingAggregate {
		msgType = hub.MessageTypeAggregate
		m.draftingAggregate = false
	}
	msg := hub.NewMessage(msgType, m.displayName, content, m.round)
	m.hub.Send(context.Background(), msg)

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
		m.hub.RequestAggregate(m.round)
	case "[Send]":
		return m.handleDirectiveSend()
	case "[New Round]":
		// Name the round being ended. The coordinator owns the next number and
		// ignores a repeat; we adopt the new round on RoundChange.
		m.hub.StartRound(m.round)
	case "[Resolve]":
		m.hub.Send(context.Background(), hub.NewResolve(m.round))
	case "[Export]":
		exportPath := "transcript_" + time.Now().Format("20060102_150405") + ".json"
		m.hub.Export(exportPath)
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
// message through the hub again.
func (m *AppModel) showAgentError(failed hub.Message) tea.Cmd {
	var retry func() tea.Cmd
	if failed.Retry != nil {
		again := *failed.Retry
		round := failed.Round
		retry = func() tea.Cmd {
			return func() tea.Msg {
				if err := m.hub.Send(context.Background(), hub.NewMessage(again.Type, hub.HostID, again.Content, round)); err != nil {
					return errorMsg(err)
				}
				return nil
			}
		}
	}
	return toast.ShowToast(failed.Content, true, retry)
}

// toggleFlag asks the coordinator to flag (or unflag) the selected
// participant. The host cannot flag themselves.
func (m *AppModel) toggleFlag() (tea.Model, tea.Cmd) {
	p, ok := m.participants.Selected()
	if !ok || p.ID == hub.HostID {
		return m, nil
	}
	var msg *hub.Message
	if m.participants.IsFlagged(p.ID) {
		msg = hub.NewUnflag(m.round, p.ID)
	} else {
		msg = hub.NewFlag(m.round, p.ID)
	}
	m.hub.Send(context.Background(), msg)
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
	round := m.round
	return toast.ShowAction(text, "Continue", func() tea.Cmd {
		return func() tea.Msg {
			m.hub.RequestAggregate(round)
			return nil
		}
	})
}
