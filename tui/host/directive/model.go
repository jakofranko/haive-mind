package directive

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	input  textarea.Model
	width  int
	height int
	styles Styles
	active bool
}

type Styles struct {
	paneBorder     lipgloss.Style
	activeBorder   lipgloss.Style
	title          lipgloss.Style
	directiveInput lipgloss.Style
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
		directiveInput: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
	}
}

func New() Model {
	input := textarea.New()
	input.Placeholder = "Type directive here..."
	input.Focus()
	input.KeyMap.InsertNewline.SetEnabled(true)
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+enter"))
	input.ShowLineNumbers = false

	return Model{
		input:  input,
		styles: defaultStyles(),
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) layout() {
	m.input.SetWidth(m.width - 2)
	m.input.SetHeight(6)
}

func (m Model) View() string {
	border := m.styles.paneBorder
	if m.active {
		border = m.styles.activeBorder
	}

	title := m.styles.title.Render("Directive (Enter=Send, Ctrl+Enter=Newline)")
	content := m.input.View()
	return border.Width(m.width).Height(m.height).Render(title + "\n" + content)
}

func (m *Model) SetActive(active bool) {
	m.active = active
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.layout()
}

func (m *Model) Value() string {
	return m.input.Value()
}

func (m *Model) SetValue(value string) {
	m.input.SetValue(value)
}

func (m *Model) Reset() {
	m.input.Reset()
}
