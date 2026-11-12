package participants

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jakofranko/haive-mind/hub"
)

type Model struct {
	participants []hub.Participant
	width        int
	height       int
	styles       Styles
	active       bool
	round        int
	cursor       int             // selected row
	flagged      map[string]bool // ids the host asked to revise
}

type Styles struct {
	paneBorder   lipgloss.Style
	activeBorder lipgloss.Style
	title        lipgloss.Style
	participant  lipgloss.Style
	statusStyles map[hub.ParticipantStatus]lipgloss.Style
	revise       lipgloss.Style
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
		revise:      lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true),
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
	case participantUpdateMsg:
		m.UpdateParticipant(hub.Participant(msg))
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			if m.cursor < len(m.participants)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		}
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
	for i, p := range m.participants {
		statusStyle := m.styles.statusStyles[p.Status]
		status := statusStyle.Render(string(p.Status))
		if m.flagged[p.ID] {
			status = m.styles.revise.Render("revise")
		}
		marker := "  "
		if m.active && i == m.cursor {
			marker = "> "
		}

		displayName := p.DisplayName
		if displayName == "" {
			displayName = p.ID
		}

		prefix := ""
		if p.ID == hub.HostID {
			prefix = "👑 "
		}
		items = append(items, m.styles.participant.Render(fmt.Sprintf("%s%s%s  %s", marker, prefix, displayName, status)))
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
		if p.ID == updated.ID {
			m.participants[i] = updated
			return
		}
	}
	// Not found - append (join)
	m.participants = append(m.participants, updated)
}

func (m *Model) RemoveParticipant(id string) {
	for i, p := range m.participants {
		if p.ID == id {
			m.participants = append(m.participants[:i], m.participants[i+1:]...)
			break
		}
	}
	if m.cursor >= len(m.participants) {
		m.cursor = len(m.participants) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) SetRound(round int) {
	m.round = round
}

func (m *Model) GetParticipants() []hub.Participant {
	return m.participants
}

// SetFlagged marks the participants the host has asked to revise.
func (m *Model) SetFlagged(ids []string) {
	m.flagged = make(map[string]bool, len(ids))
	for _, id := range ids {
		m.flagged[id] = true
	}
}

// FlaggedCount is how many participants are currently flagged.
func (m *Model) FlaggedCount() int { return len(m.flagged) }

// Selected returns the participant under the cursor.
func (m *Model) Selected() (hub.Participant, bool) {
	if m.cursor < 0 || m.cursor >= len(m.participants) {
		return hub.Participant{}, false
	}
	return m.participants[m.cursor], true
}

// IsFlagged reports whether the participant is flagged for revision.
func (m *Model) IsFlagged(id string) bool { return m.flagged[id] }
