package transcript

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"jakofranko/haive-mind/hub"
)

type Model struct {
	viewport viewport.Model
	renderer *glamour.TermRenderer
	width    int
	height   int
	styles   Styles
	active   bool
}

type Styles struct {
	paneBorder   lipgloss.Style
	activeBorder lipgloss.Style
	title        lipgloss.Style
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
	}
}

func New() Model {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	vp := viewport.New(0, 0)
	vp.SetContent("Welcome to Haive Mind Host TUI\nPress Tab to navigate panes")

	return Model{
		viewport: vp,
		renderer: renderer,
		styles:   defaultStyles(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

type messageMsg hub.Message

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, tea.Batch(cmds...)

	case messageMsg:
		m.addMessage(hub.Message(msg))
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) layout() {
	// Border (2) + title (1) + padding (2) = 5 lines overhead
	// Border left/right (2) + padding (2) = 4 cols overhead
	m.viewport.Width = m.width - 4
	m.viewport.Height = m.height - 5
	if m.viewport.Width < 10 {
		m.viewport.Width = 10
	}
	if m.viewport.Height < 5 {
		m.viewport.Height = 5
	}
}

func (m *Model) addMessage(msg hub.Message) {
	rendered, _ := m.renderer.Render(msg.Content)
	content := fmt.Sprintf("[%s] **%s** (Round %d):\n%s\n---\n",
		msg.Timestamp.Format("15:04:05"), msg.From, msg.Round, rendered)

	current := m.viewport.View()
	m.viewport.SetContent(current + content)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	border := m.styles.paneBorder
	if m.active {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render("Transcript")
	content := m.viewport.View()
	return border.Render(title + "\n" + content)
}

func (m *Model) SetActive(active bool) {
	m.active = active
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.layout()
}

func (m *Model) AddMessage(msg hub.Message) {
	rendered, _ := m.renderer.Render(msg.Content)
	content := fmt.Sprintf("[%s] **%s** (Round %d):\n%s\n---\n",
		msg.Timestamp.Format("15:04:05"), msg.From, msg.Round, rendered)

	current := m.viewport.View()
	m.viewport.SetContent(current + content)
	m.viewport.GotoBottom()
}

func (m *Model) AddSystemMessage(content string) {
	msg := fmt.Sprintf("[%s] *%s*\n---\n",
		time.Now().Format("15:04:05"), content)
	current := m.viewport.View()
	m.viewport.SetContent(current + msg)
	m.viewport.GotoBottom()
}
