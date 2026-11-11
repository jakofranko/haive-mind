package directive

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestDirectiveModel_New(t *testing.T) {
	m := New()
	assert.NotNil(t, m)
	assert.False(t, m.active)
	assert.Equal(t, "", m.Value())
}

func TestDirectiveModel_Init(t *testing.T) {
	m := New()
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestDirectiveModel_SetValueAndValue(t *testing.T) {
	m := New()
	m.SetValue("Test directive")
	assert.Equal(t, "Test directive", m.Value())
}

func TestDirectiveModel_Reset(t *testing.T) {
	m := New()
	m.SetValue("Test directive")
	m.Reset()
	assert.Equal(t, "", m.Value())
}

func TestDirectiveModel_SetActive(t *testing.T) {
	m := New()
	assert.False(t, m.active)
	m.SetActive(true)
	assert.True(t, m.active)
	m.SetActive(false)
	assert.False(t, m.active)
}

func TestDirectiveModel_SetSize(t *testing.T) {
	m := New()
	m.SetSize(80, 24)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestDirectiveModel_Focus(t *testing.T) {
	m := New()
	cmd := m.Focus()
	assert.NotNil(t, cmd)
}

func TestDirectiveModel_View_RendersWithoutPanic(t *testing.T) {
	m := New()
	m.SetSize(80, 24)

	view := m.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Directive")
}

func TestDirectiveModel_View_Active(t *testing.T) {
	m := New()
	m.SetSize(80, 24)
	m.SetActive(true)

	view := m.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Directive")
}

// The host's layout() sizes the pane through SetSize. A window size that
// reaches Update is the whole window, not the pane's column, so the pane
// draws exactly as it did before the message.
func TestDirectiveModel_KeepsItsSizeWhenAWindowSizeReachesIt(t *testing.T) {
	m := New()
	m.SetSize(40, 10)
	before := m.View()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})

	assert.Equal(t, before, updated.(Model).View(), "a window size resized the pane")
}

func TestDirectiveModel_Update_KeyMsg(t *testing.T) {
	m := New()
	m.SetSize(80, 24)
	m.SetValue("Test directive")

	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = updatedModel.(Model)
	assert.Contains(t, m.Value(), "Test directive")
}
