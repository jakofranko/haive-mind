package toast

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	message   string
	visible   bool
	width     int
	height    int
	styles    Styles
	timeout   time.Duration
	retryFunc func() tea.Cmd
}

type Styles struct {
	toast       lipgloss.Style
	toastError  lipgloss.Style
	button      lipgloss.Style
	buttonFocus lipgloss.Style
}

func defaultStyles() Styles {
	return Styles{
		toast: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("220")).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("229")).
			Padding(1, 2).
			Margin(1, 0),
		toastError: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("229")).
			Padding(1, 2).
			Margin(1, 0),
		button: lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")),
		buttonFocus: lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("39")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")),
	}
}

func New() Model {
	return Model{
		styles:  defaultStyles(),
		timeout: 5 * time.Second,
	}
}

type showToastMsg struct {
	message   string
	isError   bool
	retryFunc func() tea.Cmd
}

type hideToastMsg struct{}

func ShowToast(message string, isError bool, retryFunc func() tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		return showToastMsg{message: message, isError: isError, retryFunc: retryFunc}
	}
}

func HideToast() tea.Cmd {
	return func() tea.Msg {
		return hideToastMsg{}
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case showToastMsg:
		m.message = msg.message
		m.visible = true
		m.retryFunc = msg.retryFunc
		return m, tea.Tick(m.timeout, func(t time.Time) tea.Msg {
			return hideToastMsg{}
		})

	case hideToastMsg:
		m.visible = false
		m.retryFunc = nil
		return m, nil

	case tea.KeyMsg:
		if !m.visible {
			return m, nil
		}
		keyStr := msg.String()
		if keyStr == "enter" && m.retryFunc != nil {
			m.visible = false
			return m, m.retryFunc()
		}
		if keyStr == "esc" {
			m.visible = false
			m.retryFunc = nil
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m Model) View() string {
	if !m.visible {
		return ""
	}

	style := m.styles.toast
	if m.retryFunc != nil {
		style = m.styles.toastError
	}

	var content string
	if m.retryFunc != nil {
		btnStyle := m.styles.button
		content = style.Render(m.message + "\n\n" + btnStyle.Render("[Retry]  [Dismiss]"))
	} else {
		content = style.Render(m.message)
	}

	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, content)
}

func (m Model) IsVisible() bool {
	return m.visible
}
