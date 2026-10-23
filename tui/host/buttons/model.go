package buttons

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	buttons     []string
	buttonIndex int
	width       int
	height      int
	styles      Styles
	active      bool
}

type Styles struct {
	paneBorder   lipgloss.Style
	activeBorder lipgloss.Style
	title        lipgloss.Style
	button       lipgloss.Style
	buttonActive lipgloss.Style
}

func defaultStyles() Styles {
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
	}
}

func New() Model {
	return Model{
		buttons:     []string{"[Aggregate]", "[Send]", "[New Round]", "[Export]"},
		buttonIndex: 0,
		styles:      defaultStyles(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		keyStr := msg.String()
		switch keyStr {
		case "left", "h":
			if m.buttonIndex > 0 {
				m.buttonIndex--
			}
			return m, nil
		case "right", "l":
			if m.buttonIndex < len(m.buttons)-1 {
				m.buttonIndex++
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	border := m.styles.paneBorder
	if m.active {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render("Controls")

	var btns []string
	for i, label := range m.buttons {
		btnStyle := m.styles.button
		if i == m.buttonIndex && m.active {
			btnStyle = m.styles.buttonActive
		}
		btns = append(btns, btnStyle.Render(label))
	}

	content := strings.Join(btns, " ")
	return border.Render(title + "\n" + content)
}

func (m *Model) SetActive(active bool) {
	m.active = active
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Model) ActiveButton() string {
	if m.buttonIndex < len(m.buttons) {
		return m.buttons[m.buttonIndex]
	}
	return ""
}

func (m *Model) SetButtonIndex(index int) {
	if index >= 0 && index < len(m.buttons) {
		m.buttonIndex = index
	}
}
