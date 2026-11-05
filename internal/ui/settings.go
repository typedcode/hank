package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hoffmannma/hank/internal/config"
	"github.com/hoffmannma/hank/internal/store"
)

// focus positions in the settings form, in tab order
const (
	settingsDBPath = iota
	settingsPageSize
	settingsCancel
	settingsSave
	settingsCount
)

// settingsForm is the overlay to change the settings from hank.json.
type settingsForm struct {
	dbPath   textinput.Model
	pageSize textinput.Model
	focus    int
	err      string
}

func (m *Model) openSettings() tea.Cmd {
	f := settingsForm{
		dbPath:   newInput("default"),
		pageSize: newInput(strconv.Itoa(config.DefaultFinishedPageSize)),
	}
	if def, err := config.DefaultDBPath(); err == nil {
		f.dbPath.Placeholder = def
	}
	f.resize(m.width)

	f.dbPath.SetValue(m.cfg.DBPath)
	if m.cfg.FinishedPageSize > 0 {
		f.pageSize.SetValue(strconv.Itoa(m.cfg.FinishedPageSize))
	}
	m.settings = f
	m.mode = modeSettings
	return m.settings.setFocus(settingsDBPath)
}

func (f *settingsForm) resize(screen int) {
	w := boxInner(boxWidth(screen, formMaxWidth)) - styleField.GetHorizontalFrameSize()
	f.dbPath.Width = w - 1
	f.pageSize.Width = w - 1
}

func (f *settingsForm) setFocus(i int) tea.Cmd {
	f.focus = (i + settingsCount) % settingsCount
	f.dbPath.Blur()
	f.pageSize.Blur()
	switch f.focus {
	case settingsDBPath:
		return f.dbPath.Focus()
	case settingsPageSize:
		return f.pageSize.Focus()
	}
	return nil
}

func (f *settingsForm) updateInputs(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch f.focus {
	case settingsDBPath:
		f.dbPath, cmd = f.dbPath.Update(msg)
	case settingsPageSize:
		f.pageSize, cmd = f.pageSize.Update(msg)
	}
	return cmd
}

func (m *Model) updateSettings(msg tea.KeyMsg) tea.Cmd {
	f := &m.settings
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return nil
	case "tab":
		return f.setFocus(f.focus + 1)
	case "shift+tab":
		return f.setFocus(f.focus - 1)
	case "enter":
		if f.focus == settingsCancel {
			m.mode = modeList
			return nil
		}
		m.saveSettings()
		return nil
	}
	return f.updateInputs(msg)
}

func (m *Model) saveSettings() {
	f := &m.settings
	cfg := m.cfg
	cfg.DBPath = strings.TrimSpace(f.dbPath.Value())
	cfg.FinishedPageSize = 0
	if v := strings.TrimSpace(f.pageSize.Value()); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			f.err = "Finished page size must be a positive number."
			return
		}
		cfg.FinishedPageSize = n
	}
	if err := m.checkDBPath(cfg); err != nil {
		f.err = err.Error()
		return
	}
	if err := config.Save(cfg); err != nil {
		f.err = err.Error()
		return
	}
	m.cfg = cfg
	m.pageSize = cfg.PageSize()
	m.mode = modeList
	m.reload(0)
}

// checkDBPath makes sure a changed database path can be opened, because hank
// could not start with a broken one and the settings would be out of reach.
func (m *Model) checkDBPath(cfg config.Config) error {
	if cfg.DBPath == m.cfg.DBPath {
		return nil
	}
	path, err := cfg.ResolveDBPath()
	if err != nil {
		return err
	}
	s, err := store.Open(path)
	if err != nil {
		return fmt.Errorf("Cannot open database: %w", err)
	}
	return s.Close()
}

func (f *settingsForm) view(screen int) string {
	w := boxWidth(screen, formMaxWidth)
	fieldW := boxInner(w) - styleField.GetHorizontalFrameSize()

	parts := []string{
		styleTitle.Render("Settings"), "",
		formLabel(f.focus == settingsDBPath, "Database path (applies after restart)"),
		formField(f.focus == settingsDBPath, fieldW, f.dbPath.View()),
		formLabel(f.focus == settingsPageSize, "Finished tasks loaded at once"),
		formField(f.focus == settingsPageSize, fieldW, f.pageSize.View()),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top,
			formButton(f.focus == settingsCancel, "Cancel"), " ",
			formButton(f.focus == settingsSave, "Save")),
	}
	if f.err != "" {
		parts = append(parts, "", lipgloss.NewStyle().Width(boxInner(w)).Render(styleError.Render(f.err)))
	}
	return styleBox.Width(w - 2).Render(strings.Join(parts, "\n"))
}
