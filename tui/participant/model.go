package participant

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/inbox"
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
	hub           hub.Hub
	width         int
	height        int
	activePane    int
	round         int
	id            string // stable hub identity (set by the SSH server); never changes
	yourName      string // display name
	lastSubmitted string // what they last sent, offered back when the host asks for a revision
	flaggedSelf   bool   // the host currently has this participant flagged
	status        hub.ParticipantStatus
	sshUsername   string

	transcript transcript.Model
	sidebar    sidebar.Model
	response   response.Model
	namePrompt textinput.Model

	namePromptActive bool

	subscriptions []hub.Subscription

	// Internal channels for hub message forwarding
	msgCh         *inbox.Queue[*hub.Message]
	participantCh *inbox.Queue[*hub.Participant]
	roundCh       *inbox.Queue[int]
	errorCh       *inbox.Queue[error]

	// Synchronization
	subscriptionsReady chan struct{}
}

func NewParticipantModel(hubChans hub.Hub, yourName, sshUsername string) *ParticipantModel {
	namePrompt := textinput.New()
	namePrompt.Placeholder = "Enter display name"
	namePrompt.Focus()
	namePrompt.CharLimit = 50
	namePrompt.Prompt = "> "
	namePrompt.Width = 40

	m := &ParticipantModel{
		hub:                hubChans,
		activePane:         paneTranscript,
		round:              1,
		id:                 yourName,
		status:             hub.StatusIdle,
		yourName:           yourName,
		sshUsername:        sshUsername,
		transcript:         transcript.New(),
		sidebar:            sidebar.New(),
		response:           response.New(),
		namePrompt:         namePrompt,
		namePromptActive:   false,
		subscriptions:      make([]hub.Subscription, 0),
		msgCh:              inbox.New[*hub.Message](),
		participantCh:      inbox.New[*hub.Participant](),
		roundCh:            inbox.New[int](),
		errorCh:            inbox.New[error](),
		subscriptionsReady: make(chan struct{}),
	}

	// Pick up text a previous session left unsent.
	if draft := hubChans.GetDraft(yourName); draft != "" {
		m.response.SetDraft(draft)
	}

	m.sidebar.SetYourName(yourName)
	m.sidebar.SetParticipants([]hub.Participant{
		{Name: m.id, DisplayName: yourName, Status: hub.StatusIdle, Round: 0},
	})

	// Check if we have a stored display name
	if displayName := hubChans.GetDisplayName(sshUsername); displayName != "" {
		m.yourName = displayName
		m.sidebar.SetYourName(displayName)
		m.sidebar.SetParticipants([]hub.Participant{
			{Name: m.id, DisplayName: displayName, Status: hub.StatusIdle, Round: 0},
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
		return tea.Batch(
			textinput.Blink,
			m.setupSubscriptions(),
			m.listenForMessages(),
			m.listenForParticipants(),
			m.listenForRounds(),
			m.listenForErrors(),
			m.fetchInitialParticipants(),
		)
	}
	return tea.Batch(
		m.transcript.Init(),
		m.sidebar.Init(),
		m.response.Init(),
		m.setupSubscriptions(),
		m.listenForMessages(),
		m.listenForParticipants(),
		m.listenForRounds(),
		m.listenForErrors(),
		m.fetchInitialParticipants(),
	)
}

func (m *ParticipantModel) fetchInitialParticipants() tea.Cmd {
	return func() tea.Msg {
		participants := m.hub.GetParticipants()
		return initialParticipantsMsg(participants)
	}
}

type initialParticipantsMsg []*hub.Participant

func (m *ParticipantModel) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		msg := m.msgCh.Pop()
		return hubMessageMsg(*msg)
	}
}

func (m *ParticipantModel) listenForParticipants() tea.Cmd {
	return func() tea.Msg {
		p := m.participantCh.Pop()
		return participantUpdateMsg(p)
	}
}

func (m *ParticipantModel) listenForRounds() tea.Cmd {
	return func() tea.Msg {
		round := m.roundCh.Pop()
		return roundChangedMsg(round)
	}
}

func (m *ParticipantModel) listenForErrors() tea.Cmd {
	return func() tea.Msg {
		err := m.errorCh.Pop()
		return errorMsg(err)
	}
}

type hubMessageMsg hub.Message
type participantUpdateMsg *hub.Participant
type roundChangedMsg int
type errorMsg error

func (m *ParticipantModel) setupSubscriptions() tea.Cmd {
	ctx := context.Background()

	// Transcript traffic arrives through one subscription so that messages of
	// different types keep the order they were sent in.
	sub, err := m.hub.SubscribeTypes(ctx, []hub.MessageType{
		hub.MessageTypeDirective,
		hub.MessageTypeResponse,
		hub.MessageTypeSystem,
		hub.MessageTypeAggregate,
		hub.MessageTypeAggregateDraft,
		hub.MessageTypeConflictUpdate,
	}, func(msg *hub.Message) error {
		if msg.Type == hub.MessageTypeAggregate && msg.Content == "aggregate_request" {
			return nil // a signal to the coordinator, not transcript content
		}
		m.msgCh.Push(msg)
		return nil
	})
	if err == nil {
		m.subscriptions = append(m.subscriptions, sub)
	}

	// Subscribe to participant events
	sub, err = m.hub.Subscribe(ctx, hub.MessageTypeParticipantEvent, func(msg *hub.Message) error {
		p := m.parseParticipantEvent(msg)
		if p != nil {
			m.participantCh.Push(p)
		}
		return nil
	})
	if err == nil {
		m.subscriptions = append(m.subscriptions, sub)
	}

	// Subscribe to round changes
	sub, err = m.hub.Subscribe(ctx, hub.MessageTypeRoundChange, func(msg *hub.Message) error {
		m.roundCh.Push(msg.Round)
		return nil
	})
	if err == nil {
		m.subscriptions = append(m.subscriptions, sub)
	}

	close(m.subscriptionsReady)
	return func() tea.Msg { return subscriptionsReadyMsg{} }
}

type subscriptionsReadyMsg struct{}

func (m *ParticipantModel) handleMessage(msg *hub.Message) error {
	return nil // Kept for compatibility
}

func (m *ParticipantModel) parseParticipantEvent(msg *hub.Message) *hub.Participant {
	// The hub has already forgotten a participant who disconnected, so there is
	// nothing to look up: the event itself says who to remove.
	if msg.Content == hub.ParticipantEventDisconnect {
		return &hub.Participant{Name: msg.ID, Status: hub.StatusDisconnected, Round: msg.Round, Event: msg.Content}
	}
	participants := m.hub.GetParticipants()
	for _, p := range participants {
		if p.Name == msg.ID {
			return p
		}
	}
	return nil
}

func (m *ParticipantModel) handleParticipantUpdate(p *hub.Participant) error {
	return nil
}

func (m *ParticipantModel) handleRoundChange(round int) error {
	return nil
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
		if msg.Type == hub.MessageTypeConflictUpdate {
			m.applyFlags(msg.Content)
			return m, m.listenForMessages()
		}
		m.transcript.AddMessage(hub.Message(msg))
		cmds = append(cmds, m.listenForMessages())
		return m, tea.Batch(cmds...)

	case participantUpdateMsg:
		if msg.Status == hub.StatusDisconnected {
			m.sidebar.RemoveParticipant(msg.Name)
		} else {
			m.sidebar.UpdateParticipant(*msg)
		}
		cmds = append(cmds, m.listenForParticipants())
		return m, tea.Batch(cmds...)

	case roundChangedMsg:
		m.round = int(msg)
		m.sidebar.SetRound(m.round)
		m.setStatus(hub.StatusIdle)
		cmds = append(cmds, m.listenForRounds())
		return m, tea.Batch(cmds...)

	case initialParticipantsMsg:
		participants := make([]hub.Participant, len(msg))
		for i, p := range msg {
			participants[i] = *p
		}
		m.sidebar.SetParticipants(participants)
		return m, nil

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
					{Name: m.id, DisplayName: name, Status: hub.StatusIdle, Round: 0},
				})
				// Send participant update to hub so host panel shows this participant
				p := hub.NewParticipantEvent(m.id, name, hub.StatusIdle, m.round, hub.ParticipantEventConnect)
				m.hub.RegisterParticipant(p)
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

	msg := hub.NewMessage(hub.MessageTypeResponse, m.yourName, content, m.round)
	msg.FromID = m.id
	m.lastSubmitted = content
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

// Close releases every hub subscription this model made. Call it when the
// session ends so the hub stops delivering to a model nobody is reading.
func (m *ParticipantModel) Close() {
	for _, sub := range m.subscriptions {
		sub.Unsubscribe()
	}
	m.subscriptions = nil
}

func (m *ParticipantModel) SetYourName(name string) {
	m.yourName = name
	m.sidebar.SetYourName(name)
}

// SubscriptionsReady returns a channel that closes when subscriptions are registered
func (m *ParticipantModel) SubscriptionsReady() <-chan struct{} {
	return m.subscriptionsReady
}

// applyFlags handles the coordinator's announcement of who the host has asked
// to revise. When this participant newly becomes flagged they are told, their
// last answer comes back for editing (unless they are already typing), and
// they are idle again.
func (m *ParticipantModel) applyFlags(content string) {
	var ids []string
	if json.Unmarshal([]byte(content), &ids) != nil {
		return
	}
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
	if strings.TrimSpace(m.response.Value()) == "" && m.lastSubmitted != "" {
		m.response.SetValue(m.lastSubmitted)
		m.hub.SetDraft(m.id, m.lastSubmitted)
	}
	m.setStatus(hub.StatusIdle)
}
