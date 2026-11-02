package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hoffmannma/hank/internal/store"
)

// filter narrows the task list down to tasks matching a search text and a
// project.
type filter struct {
	projectID   int64 // optAllProjects, optNoProject or a project id
	projectName string
}

func (m *Model) storeFilter() store.Filter {
	return store.Filter{Search: m.search.Value(), ProjectID: m.filter.projectID}
}

func (m *Model) filterActive() bool {
	return strings.TrimSpace(m.search.Value()) != "" || m.filter.projectID != optAllProjects
}

// refilter reloads the tasks after search or filter changed, starting again
// with the first batch of finished tasks.
func (m *Model) refilter() {
	m.finished = nil
	m.reload(0)
}

func (m *Model) startSearch() tea.Cmd {
	m.mode = modeSearch
	m.search.CursorEnd()
	return m.search.Focus()
}

func (m *Model) updateSearch(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.search.SetValue("")
		fallthrough
	case "enter":
		m.search.Blur()
		m.mode = modeList
		m.refilter()
		return nil
	case "tab":
		m.search.Blur()
		m.openFilterPicker()
		return nil
	}
	var cmd tea.Cmd
	before := m.search.Value()
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != before {
		m.refilter()
	}
	return cmd
}

func (m *Model) clearFilter() {
	m.search.SetValue("")
	m.filter = filter{projectID: optAllProjects}
	m.refilter()
}

func (m *Model) openFilterPicker() {
	projects, err := m.store.ActiveProjects()
	if err != nil {
		m.setErr(err)
		return
	}
	first := []pickOption{
		{label: "(all projects)", id: optAllProjects},
		{label: "(no project)", id: optNoProject, style: styleMuted},
	}
	m.filterPicker = newProjectPicker("Filter by project", first, projects, false, m.filter.projectID)
	m.mode = modeFilter
}

func (m *Model) updateFilterPicker(msg tea.KeyMsg) tea.Cmd {
	choice, closed, cmd := m.filterPicker.update(msg, m.store)
	if choice != nil {
		m.filter = filter{projectID: choice.id, projectName: choice.label}
		m.refilter()
	}
	if choice != nil || closed {
		m.mode = modeList
	}
	return cmd
}

// syncFilter keeps the project filter in line with renamed, deactivated or
// deleted projects.
func (m *Model) syncFilter() {
	if m.filter.projectID <= 0 {
		return
	}
	projects, err := m.store.ActiveProjects()
	if err != nil {
		m.setErr(err)
		return
	}
	for _, p := range projects {
		if p.ID == m.filter.projectID {
			m.filter.projectName = p.Name
			return
		}
	}
	m.filter = filter{projectID: optAllProjects}
}

// viewFilterBar renders the search field and the project filter shown
// above the table.
func (m *Model) viewFilterBar() string {
	label := styleLabel.Render("  Search ")
	if m.mode == modeSearch {
		label = styleFocused.Render("› Search ")
	}
	var search string
	switch {
	case m.mode == modeSearch || m.search.Value() != "":
		m.search.Width = max(10, min(40, m.width/2))
		search = fit(m.search.View(), m.search.Width+1)
	default:
		search = styleMuted.Render(fit("press / to search", max(10, min(40, m.width/2))+1))
	}

	project := styleMuted.Render("all · press F to filter")
	if m.filter.projectID != optAllProjects {
		project = styleFocused.Render(m.filter.projectName)
	}
	return fit(label+search+"   "+styleLabel.Render("Project ")+project, m.width)
}
