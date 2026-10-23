package host

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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
	hub          *hub.HubChannels
	width        int
	height       int
	activePane   int
	buttonIndex  int
	transcript   viewport.Model
	participants []hub.Participant
	directive    string
	directiveVP  viewport.Model
	round        int
	renderer     *glamour.TermRenderer
	styles       Styles
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

	directiveVP := viewport.New(0, 0)
	directiveVP.SetContent("")

	m := &HostModel{
		hub:         hubChans,
		activePane:  paneTranscript,
		transcript:  transcriptVP,
		directiveVP: directiveVP,
		round:       1,
		renderer:    renderer,
		styles:      defaultStyles(),
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
		m.listenForEvents(),
	)
}

func (m *HostModel) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := <-m.hub.Inbound
		return msg
	}
}

func (m *HostModel) listenForEvents() tea.Cmd {
	return func() tea.Msg {
		event := <-m.hub.Events
		return event
	}
}

type hubMessageMsg hub.Message
type hubEventMsg hub.HubEvent

func (m *HostModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizePanes()
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case hubMessageMsg:
		m.addMessage(hub.Message(msg))
		cmds = append(cmds, m.listenForMessages())
		return m, tea.Batch(cmds...)

	case hubEventMsg:
		m.handleHubEvent(hub.HubEvent(msg))
		cmds = append(cmds, m.listenForEvents())
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	cmds = append(cmds, cmd)

	m.directiveVP, cmd = m.directiveVP.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *HostModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		m.activePane = (m.activePane + 1) % numPanes
		m.updateBorderStyles()
		return m, nil

	case "shift+tab":
		m.activePane = (m.activePane - 1 + numPanes) % numPanes
		m.updateBorderStyles()
		return m, nil

	case "enter":
		if m.activePane == paneDirective {
			return m.handleDirectiveSend()
		}
		if m.activePane == paneButtons {
			return m.handleButtonPress()
		}

	case "ctrl+enter":
		if m.activePane == paneDirective {
			m.directive += "\n"
			m.directiveVP.SetContent(m.directive)
			m.directiveVP.GotoBottom()
		}
		return m, nil

	case "up", "k":
		if m.activePane == paneParticipants {
			// Could add participant selection logic here
		}
		if m.activePane == paneButtons {
			// Button navigation - up does nothing for horizontal buttons
		}
		return m, nil

	case "down", "j":
		if m.activePane == paneParticipants {
			// Could add participant selection logic here
		}
		if m.activePane == paneButtons {
			// Button navigation - down does nothing for horizontal buttons
		}
		return m, nil

	case "left", "h":
		if m.activePane == paneButtons && m.buttonIndex > 0 {
			m.buttonIndex--
		}
		return m, nil

	case "right", "l":
		if m.activePane == paneButtons && m.buttonIndex < 3 {
			m.buttonIndex++
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.activePane {
	case paneTranscript:
		m.transcript, cmd = m.transcript.Update(msg)
	case paneDirective:
		m.directiveVP, cmd = m.directiveVP.Update(msg)
	}

	return m, cmd
}

func (m *HostModel) handleDirectiveSend() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.directive) == "" {
		return m, nil
	}

	msg := hub.NewMessage(hub.MessageTypeDirective, "host", m.directive, m.round)
	m.hub.Outbound <- msg
	m.addMessage(*msg)

	m.directive = ""
	m.directiveVP.SetContent("")
	return m, nil
}

func (m *HostModel) handleButtonPress() (tea.Model, tea.Cmd) {
	buttons := []string{"Aggregate", "Send", "New Round", "Export"}
	if m.activePane == paneButtons && m.buttonIndex < len(buttons) {
		switch buttons[m.buttonIndex] {
		case "Aggregate":
			m.hub.Commands <- hub.HubCommand{Type: hub.CmdAggregate, Payload: m.round}
		case "Send":
			return m.handleDirectiveSend()
		case "New Round":
			m.hub.Commands <- hub.HubCommand{Type: hub.CmdNewRound, Payload: m.round + 1}
			m.round++
		case "Export":
			m.hub.Commands <- hub.HubCommand{Type: hub.CmdExport, Payload: nil}
		}
	}
	return m, nil
}

func (m *HostModel) handleHubEvent(event hub.HubEvent) {
	switch event.Type {
	case hub.EventMessageReceived:
		if msg, ok := event.Payload.(*hub.Message); ok {
			m.addMessage(*msg)
		}
	case hub.EventParticipantUpdate:
		if p, ok := event.Payload.(hub.Participant); ok {
			m.updateParticipant(p)
		}
	case hub.EventRoundChanged:
		if round, ok := event.Payload.(int); ok {
			m.round = round
		}
	case hub.EventExportComplete:
		if path, ok := event.Payload.(string); ok {
			m.addSystemMessage(fmt.Sprintf("Exported to %s", path))
		}
	case hub.EventError:
		if err, ok := event.Payload.(error); ok {
			m.addSystemMessage(fmt.Sprintf("Error: %v", err))
		}
	}
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

func (m *HostModel) resizePanes() {
	transcriptHeight := m.height/2 - 4
	directiveHeight := 6

	m.transcript.Width = m.width/2 - 2
	m.transcript.Height = transcriptHeight

	m.directiveVP.Width = m.width/2 - 2
	m.directiveVP.Height = directiveHeight
}

func (m *HostModel) updateBorderStyles() {
	// Border styles are handled in View()
}

func (m *HostModel) View() string {
	m.updatePaneSizes()

	leftPane := m.renderTranscriptPane()
	rightTop := m.renderParticipantsPane()
	rightMid := m.renderDirectivePane()
	rightBottom := m.renderButtonsPane()

	rightPane := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightMid, rightBottom)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m *HostModel) updatePaneSizes() {
	transcriptHeight := m.height/2 - 4
	m.transcript.Width = m.width/2 - 4
	m.transcript.Height = transcriptHeight

	m.directiveVP.Width = m.width/2 - 4
	m.directiveVP.Height = 4
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
	content := m.directiveVP.View()
	if content == "" {
		content = m.styles.directiveInput.Render("Type directive here...")
	}
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

func (m *HostModel) ExportTranscript(path string) error {
	content := m.transcript.View()
	data := map[string]interface{}{
		"round":        m.round,
		"participants": m.participants,
		"transcript":   content,
		"timestamp":    time.Now(),
	}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, jsonData, 0644)
}

// HandleCtrlEnter adds a newline to the directive (for testing)
func (m *HostModel) HandleCtrlEnter() {
	m.directive += "\n"
	m.directiveVP.SetContent(m.directive)
	m.directiveVP.GotoBottom()
}
