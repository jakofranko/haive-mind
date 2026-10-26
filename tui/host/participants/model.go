package participants

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
)

type Model struct {
	participants []hub.Participant
	width        int
	height       int
	styles       Styles
	active       bool
	round        int
}

type Styles struct {
	paneBorder   lipgloss.Style
	activeBorder lipgloss.Style
	title        lipgloss.Style
	participant  lipgloss.Style
	statusStyles map[hub.ParticipantStatus]lipgloss.Style
}

func defaultStyles() Styles {
	idleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	typingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	submittedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	readyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	return Styles{
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
		participant: lipgloss.NewStyle().Padding(0, 1),
		statusStyles: map[hub.ParticipantStatus]lipgloss.Style{
			hub.StatusIdle:      idleStyle,
			hub.StatusTyping:    typingStyle,
			hub.StatusSubmitted: submittedStyle,
			hub.StatusReady:     readyStyle,
		},
	}
}

func New() Model {
	return Model{
		participants: []hub.Participant{},
		styles:       defaultStyles(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

type participantUpdateMsg hub.Participant
type roundChangedMsg int

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case participantUpdateMsg:
		m.UpdateParticipant(hub.Participant(msg))
		return m, nil

	case roundChangedMsg:
		m.round = int(msg)
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	border := m.styles.paneBorder
	if m.active {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render(fmt.Sprintf("Participants (Round %d)", m.round))

	var items []string
	for _, p := range m.participants {
		statusStyle := m.styles.statusStyles[p.Status]
		status := statusStyle.Render(string(p.Status))

		displayName := p.DisplayName
		if displayName == "" {
			displayName = p.Name
		}

		prefix := ""
		if p.Name == "host" {
			prefix = "👑 "
		}
		items = append(items, m.styles.participant.Render(fmt.Sprintf("%s%s  %s", prefix, displayName, status)))
	}

	content := strings.Join(items, "\n")
	return border.Render(title + "\n" + content)
}

func (m *Model) SetActive(active bool) {
	m.active = active
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Model) SetParticipants(participants []hub.Participant) {
	m.participants = participants
}

func (m *Model) UpdateParticipant(updated hub.Participant) {
	for i, p := range m.participants {
		if p.Name == updated.Name {
			m.participants[i] = updated
			return
		}
	}
	// Not found - append (join)
	m.participants = append(m.participants, updated)
}

func (m *Model) RemoveParticipant(name string) {
	for i, p := range m.participants {
		if p.Name == name {
			m.participants = append(m.participants[:i], m.participants[i+1:]...)
			break
		}
	}
}

func (m *Model) SetRound(round int) {
	m.round = round
}

func (m *Model) GetParticipants() []hub.Participant {
	return m.participants
}
