package toast

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToastModel_Init(t *testing.T) {
	m := New()
	assert.NotNil(t, m)
	assert.False(t, m.IsVisible())
}

func TestToastModel_ShowToast(t *testing.T) {
	m := New()

	cmd := ShowToast("Test message", false, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)

	assert.True(t, m.IsVisible())
}

func TestToastModel_ShowErrorToast(t *testing.T) {
	m := New()

	cmd := ShowToast("Error message", true, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)

	assert.True(t, m.IsVisible())
}

func TestToastModel_HideToast(t *testing.T) {
	m := New()

	// Show toast first
	cmd := ShowToast("Test message", false, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)
	assert.True(t, m.IsVisible())

	// Hide toast
	cmd = HideToast()
	updatedModel, _ = m.Update(cmd())
	m = updatedModel.(Model)

	assert.False(t, m.IsVisible())
}

func TestToastModel_RetryFunc(t *testing.T) {
	m := New()
	retryCalled := false

	retryFunc := func() tea.Cmd {
		retryCalled = true
		return nil
	}

	cmd := ShowToast("Error message", true, retryFunc)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)

	// Simulate Enter key press
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updatedModel.(Model)

	assert.True(t, retryCalled)
	assert.False(t, m.IsVisible())
}

func TestToastModel_DismissWithEsc(t *testing.T) {
	m := New()

	cmd := ShowToast("Error message", true, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)

	// Simulate Esc key press
	updatedModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updatedModel.(Model)

	assert.False(t, m.IsVisible())
}

func TestToastModel_AutoHide(t *testing.T) {
	m := New()

	cmd := ShowToast("Test message", false, nil)
	updatedModel, cmdResult := m.Update(cmd())
	m = updatedModel.(Model)

	assert.True(t, m.IsVisible())
	assert.NotNil(t, cmdResult)

	// We can't easily test the auto-hide without more complex setup
	// since it uses tea.Tick internally
	_ = cmdResult
}

func TestToastModel_View_WhenHidden(t *testing.T) {
	m := New()
	m.SetSize(80, 24)

	view := m.View()
	assert.Empty(t, view)
}

func TestToastModel_View_WhenVisible(t *testing.T) {
	m := New()

	cmd := ShowToast("Test message", false, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)
	m.SetSize(80, 24)

	view := m.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Test message")
}

func TestToastModel_View_ErrorToast(t *testing.T) {
	m := New()

	cmd := ShowToast("Error message", true, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)
	m.SetSize(80, 24)

	view := m.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Error message")
}

func TestToastModel_SetSize(t *testing.T) {
	m := New()
	m.SetSize(100, 30)

	// Should not panic
	cmd := ShowToast("Test", false, nil)
	updatedModel, _ := m.Update(cmd())
	m = updatedModel.(Model)
	view := m.View()
	assert.NotEmpty(t, view)
}
