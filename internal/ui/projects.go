package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hoffmannma/hank/internal/store"
)

const projectsMaxWidth = 56

// projectsView is the overlay to manage projects.
type projectsView struct {
	items  []store.Project
	cursor int

	editing bool
	editID  int64 // 0 while creating a new project
	input   textinput.Model
	err     string
}

func (p *projectsView) resize(screen int) {
	p.input.Width = boxInner(boxWidth(screen, projectsMaxWidth)) - styleField.GetHorizontalFrameSize() - 1
}

func (m *Model) openProjects() {
	m.projects = projectsView{input: newInput("project name")}
	m.projects.resize(m.width)
	m.reloadProjects(0)
	m.mode = modeProjects
}

// reloadProjects refreshes the project list (and the task list, whose
// visibility depends on the projects) keeping the cursor on selectID.
func (m *Model) reloadProjects(selectID int64) {
	p := &m.projects
	if selectID == 0 && p.cursor < len(p.items) {
		selectID = p.items[p.cursor].ID
	}
	items, err := m.store.ListProjects()
	if err != nil {
		m.setErr(err)
		return
	}
	p.items = items
	for i, pr := range items {
		if pr.ID == selectID {
			p.cursor = i
		}
	}
	p.cursor = max(0, min(p.cursor, len(items)-1))
	m.syncFilter()
	m.reload(0)
}

func (m *Model) updateProjects(msg tea.KeyMsg) tea.Cmd {
	p := &m.projects
	if p.editing {
		switch msg.String() {
		case "esc":
			p.editing = false
			p.err = ""
			p.input.Blur()
			return nil
		case "enter":
			id, err := p.editID, error(nil)
			if id == 0 {
				id, err = m.store.CreateProject(p.input.Value())
			} else {
				err = m.store.RenameProject(id, p.input.Value())
			}
			if err != nil {
				p.err = err.Error()
				return nil
			}
			p.editing = false
			p.err = ""
			p.input.Blur()
			m.reloadProjects(id)
			return nil
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}

	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		return nil
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
		return nil
	case "down", "j":
		p.cursor = max(0, min(len(p.items)-1, p.cursor+1))
		return nil
	case "c":
		return p.startEdit(0, "")
	}
	if len(p.items) == 0 {
		return nil
	}
	pr := p.items[p.cursor]
	switch msg.String() {
	case "e":
		return p.startEdit(pr.ID, pr.Name)
	case "a":
		m.confirmToggleActive(pr)
	case "d":
		m.confirmDeleteProject(pr)
	}
	return nil
}

func (p *projectsView) startEdit(id int64, name string) tea.Cmd {
	p.editing = true
	p.editID = id
	p.err = ""
	p.input.SetValue(name)
	p.input.CursorEnd()
	return p.input.Focus()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func (m *Model) confirmToggleActive(pr store.Project) {
	apply := func(m *Model) {
		m.setErr(m.store.SetProjectActive(pr.ID, !pr.Active))
		m.reloadProjects(pr.ID)
	}
	if pr.Active {
		open, err := m.store.OpenTaskCount(pr.ID)
		if err != nil {
			m.setErr(err)
			return
		}
		msg := fmt.Sprintf("Deactivate project %q?", pr.Name)
		if open > 0 {
			msg = fmt.Sprintf("Project %q still has %s.\nThey will be hidden while the project is inactive.\n\nDeactivate it anyway?",
				pr.Name, plural(open, "open task"))
		}
		m.askConfirm(msg, apply)
		return
	}
	m.askConfirm(fmt.Sprintf("Activate project %q?", pr.Name), apply)
}

func (m *Model) confirmDeleteProject(pr store.Project) {
	remove := func(m *Model) {
		m.setErr(m.store.DeleteProject(pr.ID))
		m.reloadProjects(0)
	}
	m.askConfirm(fmt.Sprintf("Delete project %q?", pr.Name), func(m *Model) {
		open, err := m.store.OpenTaskCount(pr.ID)
		if err != nil {
			m.setErr(err)
			return
		}
		if open == 0 {
			remove(m)
			return
		}
		m.askConfirm(fmt.Sprintf("Project %q still has %s.\nThey will be removed as well.\n\nDelete it anyway?",
			pr.Name, plural(open, "open task")), remove)
	})
}

func (p *projectsView) view(screen int) string {
	w := boxWidth(screen, projectsMaxWidth)
	inner := boxInner(w)
	parts := []string{styleTitle.Render("Projects"), ""}

	if len(p.items) == 0 {
		parts = append(parts, styleMuted.Render("No projects yet. Press c to create one."))
	}
	const statusW = 8
	for i, pr := range p.items {
		style := lipgloss.NewStyle()
		status := "active"
		if !pr.Active {
			style = style.Foreground(colorFinished)
			status = "inactive"
		}
		marker := "  "
		if i == p.cursor {
			style = style.Background(colorSelected)
			marker = "› "
		}
		parts = append(parts, style.Render(marker+fit(pr.Name, inner-2-statusW-1)+" "+fit(status, statusW)))
	}

	if p.editing {
		heading := "New project"
		if p.editID != 0 {
			heading = "Rename project"
		}
		parts = append(parts, "", styleLabel.Render(heading),
			styleFieldFocused.Width(inner-styleField.GetHorizontalFrameSize()).Render(p.input.View()))
	}
	if p.err != "" {
		parts = append(parts, "", styleError.Render(p.err))
	}
	return styleBox.Width(w - 2).Render(strings.Join(parts, "\n"))
}
