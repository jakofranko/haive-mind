package host

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
)

const (
	paneTranscript = iota
	paneParticipants
	paneDirective
	paneButtons
	numPanes
)

type HostModel struct {
	hub            *hub.HubChannels
	width          int
	height         int
	activePane     int
	buttonIndex    int
	transcript     viewport.Model
	participants   []hub.Participant
	directiveInput textarea.Model
	round          int
	renderer       *glamour.TermRenderer
	styles         Styles
}

type Styles struct {
	base            lipgloss.Style
	paneBorder      lipgloss.Style
	activeBorder    lipgloss.Style
	title           lipgloss.Style
	participant     lipgloss.Style
	statusIdle      lipgloss.Style
	statusTyping    lipgloss.Style
	statusSubmitted lipgloss.Style
	statusReady     lipgloss.Style
	button          lipgloss.Style
	buttonActive    lipgloss.Style
	directiveInput  lipgloss.Style
}

func defaultStyles() Styles {
	return Styles{
		base: lipgloss.NewStyle().
			Padding(0, 1),
		paneBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),
		activeBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Padding(0, 1),
		participant: lipgloss.NewStyle().
			Padding(0, 1),
		statusIdle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
		statusTyping: lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")),
		statusSubmitted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")),
		statusReady: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")),
		button: lipgloss.NewStyle().
			Padding(0, 2).
			Margin(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")),
		buttonActive: lipgloss.NewStyle().
			Padding(0, 2).
			Margin(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")).
			Background(lipgloss.Color("39")).
			Foreground(lipgloss.Color("0")),
		directiveInput: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
	}
}

func NewHostModel(hubChans *hub.HubChannels) *HostModel {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	transcriptVP := viewport.New(0, 0)
	transcriptVP.SetContent("Welcome to Haive Mind Host TUI\nPress Tab to navigate panes")

	directiveInput := textarea.New()
	directiveInput.Placeholder = "Type directive here..."
	directiveInput.Focus()
	directiveInput.KeyMap.InsertNewline.SetEnabled(true)
	directiveInput.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+enter"))
	directiveInput.ShowLineNumbers = false

	m := &HostModel{
		hub:            hubChans,
		activePane:     paneTranscript,
		transcript:     transcriptVP,
		directiveInput: directiveInput,
		round:          1,
		renderer:       renderer,
		styles:         defaultStyles(),
		participants: []hub.Participant{
			{Name: "Agent-1", Status: hub.StatusIdle, Round: 0},
			{Name: "Agent-2", Status: hub.StatusIdle, Round: 0},
			{Name: "Agent-3", Status: hub.StatusIdle, Round: 0},
		},
	}

	return m
}

func (m *HostModel) Init() tea.Cmd {
	return tea.Batch(
		m.listenForMessages(),
		m.listenForParticipantUpdates(),
		m.listenForRoundChanges(),
		m.listenForExportComplete(),
		m.listenForErrors(),
	)
}

func (m *HostModel) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.hub.Inbound
		return msg
	}
}

func (m *HostModel) listenForParticipantUpdates() tea.Cmd {
	return func() tea.Msg {
		p := <-m.hub.ParticipantUpdateChan
		return participantUpdateMsg(p)
	}
}

func (m *HostModel) listenForRoundChanges() tea.Cmd {
	return func() tea.Msg {
		round := <-m.hub.RoundChangedChan
		return roundChangedMsg(round)
	}
}

func (m *HostModel) listenForExportComplete() tea.Cmd {
	return func() tea.Msg {
		path := <-m.hub.ExportCompleteChan
		return exportCompleteMsg(path)
	}
}

func (m *HostModel) listenForErrors() tea.Cmd {
	return func() tea.Msg {
		err := <-m.hub.ErrorChan
		return errorMsg(err)
	}
}

type hubMessageMsg hub.Message

// Typed event messages
type participantUpdateMsg hub.Participant
type roundChangedMsg int
type exportCompleteMsg string
type errorMsg error

func (m *HostModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case hubMessageMsg:
		m.addMessage(hub.Message(msg))
		cmds = append(cmds, m.listenForMessages())
		return m, tea.Batch(cmds...)

	case participantUpdateMsg:
		m.updateParticipant(hub.Participant(msg))
		cmds = append(cmds, m.listenForParticipantUpdates())
		return m, tea.Batch(cmds...)

	case roundChangedMsg:
		m.round = int(msg)
		cmds = append(cmds, m.listenForRoundChanges())
		return m, tea.Batch(cmds...)

	case exportCompleteMsg:
		m.addSystemMessage("Exported to " + string(msg))
		cmds = append(cmds, m.listenForExportComplete())
		return m, tea.Batch(cmds...)

	case errorMsg:
		m.addSystemMessage("Error: " + msg.Error())
		cmds = append(cmds, m.listenForErrors())
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)

	// Always update directive input (it handles its own focus)
	m.directiveInput, cmd = m.directiveInput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *HostModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Global pane navigation
	switch keyStr {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		m.activePane = (m.activePane + 1) % numPanes
		return m, nil

	case "shift+tab":
		m.activePane = (m.activePane - 1 + numPanes) % numPanes
		return m, nil
	}

	// Pane-specific handling
	switch m.activePane {
	case paneDirective:
		// Intercept Enter (without Ctrl) to send directive
		// Let textarea handle all other keys including Ctrl+Enter for newlines
		if keyStr == "enter" && !isCtrlPressed(msg) {
			return m.handleDirectiveSend()
		}
		// Let textarea handle the key
		var cmd tea.Cmd
		m.directiveInput, cmd = m.directiveInput.Update(msg)
		return m, cmd

	case paneButtons:
		if keyStr == "enter" {
			return m.handleButtonPress()
		}
		return m, nil

	case paneTranscript:
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd

	case paneParticipants:
		// Participants pane is read-only for now
		return m, nil
	}

	return m, nil
}

// isCtrlPressed checks if Ctrl modifier is held (for Enter vs Ctrl+Enter)
func isCtrlPressed(msg tea.KeyMsg) bool {
	// Check if the key message has Ctrl modifier
	// tea.KeyMsg doesn't expose modifiers directly, so we check the string
	// For Ctrl+Enter, the string representation is typically "ctrl+enter"
	return strings.HasPrefix(msg.String(), "ctrl+")
}

func (m *HostModel) handleDirectiveSend() (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(m.directiveInput.Value())
	if content == "" {
		return m, nil
	}

	msg := hub.NewMessage(hub.MessageTypeDirective, "host", content, m.round)
	m.hub.Outbound <- msg // For local transcript display
	m.hub.SendChan <- msg // For hub to send to agents
	m.addMessage(*msg)

	m.directiveInput.Reset()
	return m, nil
}

func (m *HostModel) handleButtonPress() (tea.Model, tea.Cmd) {
	buttons := []string{"Aggregate", "Send", "New Round", "Export"}
	if m.activePane == paneButtons && m.buttonIndex < len(buttons) {
		switch buttons[m.buttonIndex] {
		case "Aggregate":
			m.hub.AggregateChan <- m.round
		case "Send":
			return m.handleDirectiveSend()
		case "New Round":
			m.round++
			m.hub.NewRoundChan <- m.round
		case "Export":
			// Generate export path with timestamp
			exportPath := "transcript_" + time.Now().Format("20060102_150405") + ".json"
			m.hub.ExportChan <- exportPath
		}
	}
	return m, nil
}

func (m *HostModel) addMessage(msg hub.Message) {
	rendered, _ := m.renderer.Render(msg.Content)
	content := fmt.Sprintf("[%s] **%s** (Round %d):\n%s\n---\n",
		msg.Timestamp.Format("15:04:05"), msg.From, msg.Round, rendered)

	current := m.transcript.View()
	m.transcript.SetContent(current + content)
	m.transcript.GotoBottom()
}

func (m *HostModel) addSystemMessage(content string) {
	msg := fmt.Sprintf("[%s] *%s*\n---\n",
		time.Now().Format("15:04:05"), content)
	current := m.transcript.View()
	m.transcript.SetContent(current + msg)
	m.transcript.GotoBottom()
}

func (m *HostModel) updateParticipant(updated hub.Participant) {
	for i, p := range m.participants {
		if p.Name == updated.Name {
			m.participants[i] = updated
			break
		}
	}
}

func (m *HostModel) layout() {
	transcriptHeight := m.height/2 - 4
	directiveHeight := 6

	m.transcript.Width = m.width/2 - 2
	m.transcript.Height = transcriptHeight

	m.directiveInput.SetWidth(m.width/2 - 2)
	m.directiveInput.SetHeight(directiveHeight)
}

func (m *HostModel) View() string {
	m.layout()

	leftPane := m.renderTranscriptPane()
	rightTop := m.renderParticipantsPane()
	rightMid := m.renderDirectivePane()
	rightBottom := m.renderButtonsPane()

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightMid, rightBottom)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m *HostModel) renderTranscriptPane() string {
	border := m.styles.paneBorder
	if m.activePane == paneTranscript {
		border = m.styles.activeBorder
	}
	title := m.styles.title.Render("Transcript")
	content := m.transcript.View()
	return border.Width(m.width/2 - 2).Height(m.height/2 - 2).Render(title + "\n" + content)
}

func (m *HostModel) renderParticipantsPane() string {
	border := m.styles.paneBorder
	if m.activePane == paneParticipants {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render(fmt.Sprintf("Participants (Round %d)", m.round))

	var items []string
	for _, p := range m.participants {
		statusStyle := m.styles.statusIdle
		switch p.Status {
		case hub.StatusIdle:
			statusStyle = m.styles.statusIdle
		case hub.StatusTyping:
			statusStyle = m.styles.statusTyping
		case hub.StatusSubmitted:
			statusStyle = m.styles.statusSubmitted
		case hub.StatusReady:
			statusStyle = m.styles.statusReady
		}
		status := statusStyle.Render(string(p.Status))
		items = append(items, m.styles.participant.Render(fmt.Sprintf("%s  %s", p.Name, status)))
	}

	content := strings.Join(items, "\n")
	return border.Width(m.width/2 - 2).Height(m.height/2 - 2).Render(title + "\n" + content)
}

func (m *HostModel) renderDirectivePane() string {
	border := m.styles.directiveInput
	if m.activePane == paneDirective {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render("Directive (Enter=Send, Ctrl+Enter=Newline)")
	content := m.directiveInput.View()
	return border.Width(m.width/2 - 2).Height(8).Render(title + "\n" + content)
}

func (m *HostModel) renderButtonsPane() string {
	border := m.styles.paneBorder
	if m.activePane == paneButtons {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render("Controls")

	buttonLabels := []string{"[Aggregate]", "[Send]", "[New Round]", "[Export]"}
	var buttons []string
	for i, label := range buttonLabels {
		btnStyle := m.styles.button
		if i == m.buttonIndex && m.activePane == paneButtons {
			btnStyle = m.styles.buttonActive
		}
		buttons = append(buttons, btnStyle.Render(label))
	}

	content := strings.Join(buttons, " ")
	return border.Width(m.width/2 - 2).Height(6).Render(title + "\n" + content)
}
