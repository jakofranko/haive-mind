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
	content  string // full history; viewport.View() only returns the visible lines
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
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"), // fixed: auto-detection would read the server's terminal, not the SSH user's
		glamour.WithWordWrap(80),
	)
	if err != nil {
		renderer = nil
	}

	vp := viewport.New(0, 0)
	const welcome = "Welcome to Haive Mind Participant TUI\nWaiting for messages..."
	vp.SetContent(welcome)

	return Model{
		viewport: vp,
		content: welcome + `
`,
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
	rendered := m.renderMarkdown(msg.Content)
	content := fmt.Sprintf("[%s] %s (Round %d):\n%s\n---\n",
		msg.Timestamp.Format("15:04:05"), msg.From, msg.Round, rendered)

	m.appendContent(content)
}

// appendContent adds text to the full history and shows the newest end.
func (m *Model) appendContent(text string) {
	m.content += text
	m.viewport.SetContent(m.content)
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
	rendered := m.renderMarkdown(msg.Content)
	content := fmt.Sprintf("[%s] %s (Round %d):\n%s\n---\n",
		msg.Timestamp.Format("15:04:05"), msg.From, msg.Round, rendered)

	m.appendContent(content)
}

func (m *Model) AddSystemMessage(content string) {
	msg := fmt.Sprintf("[%s] *%s*\n---\n",
		time.Now().Format("15:04:05"), content)
	m.appendContent(msg)
}

func (m *Model) GetContent() string {
	return m.viewport.View()
}

func (m *Model) SetContent(content string) {
	m.content = content
	m.viewport.SetContent(content)
}

// renderMarkdown renders text as Markdown, falling back to the raw text if the
// renderer is unavailable or fails.
func (m *Model) renderMarkdown(text string) string {
	if m.renderer == nil {
		return text
	}
	out, err := m.renderer.Render(text)
	if err != nil {
		return text
	}
	return out
}
