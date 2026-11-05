// Package ui implements the hank terminal user interface.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hoffmannma/hank/internal/config"
	"github.com/hoffmannma/hank/internal/store"
)

type mode int

const (
	modeList mode = iota
	modeDetail
	modeForm
	modeProjects
	modeConfirm
	modeSearch
	modeFilter
	modeSettings
)

// confirmDialog asks a y/n question on top of the view it returns to.
type confirmDialog struct {
	message string
	back    mode
	onYes   func(m *Model)
}

// Model is the Bubble Tea model of hank.
type Model struct {
	store *store.Store
	cfg   config.Config
	now   func() time.Time

	width, height int
	mode          mode
	status        string // error shown in place of the help line

	open, finished []store.Task // visible tasks, after search and filter
	finishedMore   bool         // more finished tasks can be loaded
	pageSize       int          // finished tasks loaded at once
	cursor         int
	offset         int

	search       textinput.Model
	filter       filter
	filterPicker projectPicker

	form     taskForm
	projects projectsView
	settings settingsForm
	confirm  confirmDialog
}

// New creates the model and loads the task list.
func New(s *store.Store, cfg config.Config) *Model {
	m := &Model{store: s, cfg: cfg, now: time.Now, width: 80, height: 24, pageSize: cfg.PageSize(),
		search: newInput(""), filter: filter{projectID: optAllProjects}}
	m.reload(0)
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) setErr(err error) {
	if err != nil {
		m.status = err.Error()
	}
}

// reload refreshes the task list matching search and filter, keeping as
// many finished tasks loaded as before, and puts the cursor on the task with
// id selectID if it is still visible.
func (m *Model) reload(selectID int64) {
	if selectID == 0 {
		if t, ok := m.selected(); ok {
			selectID = t.ID
		}
	}
	f := m.storeFilter()
	open, err := m.store.ListOpen(f)
	if err != nil {
		m.setErr(err)
		return
	}
	finished, more, err := m.store.ListFinished(f, 0, max(m.pageSize, len(m.finished)))
	if err != nil {
		m.setErr(err)
		return
	}
	m.open, m.finished, m.finishedMore = open, finished, more
	for i, t := range m.tasks() {
		if t.ID == selectID {
			m.cursor = i
			return
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.open)+len(m.finished)-1))
}

// loadMore fetches the next batch of finished tasks once the cursor gets
// close to the last loaded one.
func (m *Model) loadMore() {
	const ahead = 5
	if !m.finishedMore || m.cursor < len(m.open)+len(m.finished)-ahead {
		return
	}
	more, hasMore, err := m.store.ListFinished(m.storeFilter(), len(m.finished), m.pageSize)
	if err != nil {
		m.setErr(err)
		return
	}
	m.finished = append(m.finished, more...)
	m.finishedMore = hasMore
}

func (m *Model) tasks() []store.Task {
	return append(append([]store.Task{}, m.open...), m.finished...)
}

func (m *Model) selected() (store.Task, bool) {
	switch {
	case m.cursor < len(m.open):
		return m.open[m.cursor], true
	case m.cursor-len(m.open) < len(m.finished):
		return m.finished[m.cursor-len(m.open)], true
	}
	return store.Task{}, false
}

func (m *Model) askConfirm(message string, onYes func(m *Model)) {
	back := m.mode
	if back == modeConfirm {
		back = m.confirm.back
	}
	m.confirm = confirmDialog{message: message, back: back, onYes: onYes}
	m.mode = modeConfirm
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.mode == modeForm {
			m.form.resize(m.width)
		}
		if m.mode == modeSettings {
			m.settings.resize(m.width)
		}
		m.projects.resize(m.width)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.status = ""
		switch m.mode {
		case modeList:
			return m, m.updateList(msg)
		case modeDetail:
			switch msg.String() {
			case "esc", "enter", "q":
				m.mode = modeList
			}
			return m, nil
		case modeForm:
			return m, m.updateForm(msg)
		case modeProjects:
			return m, m.updateProjects(msg)
		case modeSearch:
			return m, m.updateSearch(msg)
		case modeFilter:
			return m, m.updateFilterPicker(msg)
		case modeSettings:
			return m, m.updateSettings(msg)
		case modeConfirm:
			switch msg.String() {
			case "y", "Y":
				m.mode = m.confirm.back
				m.confirm.onYes(m)
			case "n", "N", "esc", "q":
				m.mode = m.confirm.back
			}
			return m, nil
		}
	}
	// Forward everything else (e.g. cursor blinking) to the active inputs.
	switch m.mode {
	case modeForm:
		return m, m.form.updateInputs(msg)
	case modeSettings:
		return m, m.settings.updateInputs(msg)
	case modeProjects:
		var cmd tea.Cmd
		m.projects.input, cmd = m.projects.input.Update(msg)
		return m, cmd
	case modeSearch:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateList(msg tea.KeyMsg) tea.Cmd {
	count := len(m.open) + len(m.finished)
	task, ok := m.selected()
	switch msg.String() {
	case "q":
		return tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = max(0, min(count-1, m.cursor+1))
		m.loadMore()
	case "c":
		return m.openForm(store.Task{})
	case "p":
		m.openProjects()
	case "s":
		return m.openSettings()
	case "/":
		return m.startSearch()
	case "F":
		m.openFilterPicker()
	case "esc":
		if m.filterActive() {
			m.clearFilter()
		}
	}
	if !ok {
		return nil
	}
	switch msg.String() {
	case "+", "-":
		// swap with the visible neighbour, so moving works in a filtered list
		other := m.cursor + 1
		if msg.String() == "+" {
			other = m.cursor - 1
		}
		if task.State != store.Finished && other >= 0 && other < len(m.open) {
			m.setErr(m.store.SwapPositions(task.ID, m.open[other].ID))
			m.reload(task.ID)
		}
	case " ":
		if task.State != store.Finished {
			m.setErr(m.store.ToggleStarted(task.ID))
			m.reload(task.ID)
		}
	case "f":
		m.setErr(m.store.ToggleFinished(task.ID, m.now()))
		if task.State == store.Finished {
			m.reload(task.ID) // reopened: keep it selected
			break
		}
		// finished: stay in the open tasks and select the one that followed
		// it, or the one before if it was the last
		var next int64
		if m.cursor+1 < len(m.open) {
			next = m.open[m.cursor+1].ID
		} else if m.cursor > 0 {
			next = m.open[m.cursor-1].ID
		}
		m.reload(next)
	case "d":
		m.askConfirm(fmt.Sprintf("Delete task %q?", task.Title), func(m *Model) {
			m.setErr(m.store.DeleteTask(task.ID))
			m.reload(0)
		})
	case "e":
		return m.openForm(task)
	case "enter":
		m.mode = modeDetail
	}
	return nil
}

// View

func (m *Model) View() string {
	return m.viewFor(m.mode)
}

func (m *Model) viewFor(md mode) string {
	switch md {
	case modeDetail:
		return m.overlay(m.viewList(), m.viewDetail())
	case modeForm:
		base := m.overlay(m.viewList(), m.form.view(m.width))
		if m.form.picking {
			return m.overlay(base, m.form.picker.view(m.width))
		}
		return base
	case modeProjects:
		return m.overlay(m.viewList(), m.projects.view(m.width))
	case modeFilter:
		return m.overlay(m.viewList(), m.filterPicker.view(m.width))
	case modeSettings:
		return m.overlay(m.viewList(), m.settings.view(m.width))
	case modeConfirm:
		inner := min(lipgloss.Width(m.confirm.message), boxInner(boxWidth(m.width, 64)))
		msg := lipgloss.NewStyle().Width(inner).Render(m.confirm.message)
		box := styleBox.Render(msg + "\n\n" + styleMuted.Render("y yes · n no"))
		return m.overlay(m.viewFor(m.confirm.back), box)
	}
	return m.viewList()
}

func (m *Model) overlay(bg, box string) string {
	return placeOverlay(dim(bg), box, m.width, m.height)
}

func (m *Model) helpLine() string {
	if m.status != "" {
		return styleError.Render(fit(m.status, m.width))
	}
	var help string
	switch m.mode {
	case modeList:
		help = "↑/k ↓/j select · +/- move · space start/stop · f finish · c create · e edit · d delete · enter details · / search · F filter · p projects · s settings · q quit"
		if m.filterActive() {
			help = "esc clear search/filter · " + help
		}
	case modeDetail:
		help = "esc/enter close"
	case modeForm:
		help = "tab/shift+tab next/previous field · enter in title saves · enter select project / press button · esc cancel"
		if m.form.picking {
			help = "↑/k ↓/j select · enter choose · esc back"
		}
	case modeProjects:
		help = "↑/k ↓/j select · c create · e rename · a activate/deactivate · d delete · esc/q close"
		if m.projects.editing {
			help = "enter save · esc cancel"
		}
	case modeConfirm:
		help = "y yes · n no"
	case modeSearch:
		help = "type to search title and text · tab project filter · enter done · esc clear"
	case modeFilter:
		help = "↑/k ↓/j select · enter choose · esc back"
	case modeSettings:
		help = "tab/shift+tab next/previous field · enter save · esc cancel"
	}
	return styleHelp.Render(fit(help, m.width))
}

func (m *Model) isOverdue(t store.Task) bool {
	return t.State != store.Finished && t.Due != "" && t.Due < m.now().Format(dateLayout)
}

func (m *Model) viewList() string {
	// project and date columns are as narrow as their content, the title
	// column takes the remaining width
	projW := len("Project")
	for _, t := range m.tasks() {
		projW = max(projW, lipgloss.Width(t.ProjectName))
	}
	projW = min(projW, 20)
	const dateW, infoW, gap = 10, 1, "  "
	titleW := max(10, m.width-2-projW-infoW-dateW-3*len(gap))

	row := func(i int, t store.Task) string {
		style := lipgloss.NewStyle()
		switch {
		case t.State == store.Started:
			style = style.Foreground(colorStarted).Bold(true)
		case t.State == store.Finished:
			style = style.Foreground(colorFinished)
		case m.isOverdue(t):
			style = style.Foreground(colorOverdue)
		}
		marker := "  "
		if i == m.cursor {
			style = style.Background(colorSelected)
			marker = "› "
		}
		info := " "
		if t.Text != "" {
			info = "ⓘ"
		}
		date, dateStyle := t.Due, style
		if t.State == store.Finished {
			date = t.FinishedAt.Local().Format(dateLayout)
		} else if m.isOverdue(t) {
			// started tasks stay green, only their date turns red
			dateStyle = dateStyle.Foreground(colorOverdue)
		}
		return style.Render(marker+fit(t.ProjectName, projW)+gap+fit(info, infoW)+gap+fit(t.Title, titleW)+gap) +
			dateStyle.Render(fit(date, dateW))
	}

	header := styleHeader.Render("  " + fit("Project", projW) + gap + fit(" ", infoW) + gap + fit("Title", titleW) + gap + fit("Due", dateW))

	// body lines, with the line index of the selected task
	var lines []string
	cursorLine := 0
	if len(m.open)+len(m.finished) == 0 {
		msg := "  No tasks yet. Press c to create one."
		if m.filterActive() {
			msg = "  No matching tasks. Press esc to clear search and filter."
		}
		lines = append(lines, "", styleMuted.Render(msg))
	}
	for i, t := range m.open {
		if i == m.cursor {
			cursorLine = len(lines)
		}
		lines = append(lines, row(i, t))
	}
	if len(m.finished) > 0 {
		lines = append(lines, "", styleMuted.Render("  "+fit("", projW)+gap+fit("", infoW)+gap+fit("Finished", titleW)+gap+fit("Finished", dateW)))
		for i, t := range m.finished {
			idx := len(m.open) + i
			if idx == m.cursor {
				cursorLine = len(lines)
			}
			lines = append(lines, row(idx, t))
		}
		if m.finishedMore {
			lines = append(lines, styleMuted.Render("  "+fit("", projW)+gap+fit("", infoW)+gap+"… more finished tasks, move down to load them"))
		}
	}

	// scroll so the selected task stays visible
	avail := max(1, m.height-4)
	if cursorLine < m.offset {
		m.offset = cursorLine
	} else if cursorLine >= m.offset+avail {
		m.offset = cursorLine - avail + 1
	}
	m.offset = max(0, min(m.offset, len(lines)-avail))
	end := min(len(lines), m.offset+avail)
	body := lines[m.offset:end]
	for len(body) < avail {
		body = append(body, "")
	}

	return m.viewFilterBar() + "\n\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.helpLine()
}

func (m *Model) viewDetail() string {
	t, _ := m.selected()
	w := boxWidth(m.width, 72)
	inner := boxInner(w)

	field := func(label, value string) string {
		return styleLabel.Render(fit(label, 10)) + value
	}
	var state string
	switch t.State {
	case store.NotStarted:
		state = "not started"
	case store.Started:
		state = lipgloss.NewStyle().Foreground(colorStarted).Bold(true).Render("started")
	case store.Finished:
		state = styleMuted.Render("finished")
	}
	lines := []string{styleTitle.Render(t.Title), ""}
	if t.ProjectName != "" {
		lines = append(lines, field("Project", t.ProjectName))
	}
	if t.Due != "" {
		due := t.Due
		if m.isOverdue(t) {
			due = styleError.Render(due + " (overdue)")
		}
		lines = append(lines, field("Due", due))
	}
	lines = append(lines, field("State", state))
	if t.State == store.Finished {
		lines = append(lines, field("Finished", t.FinishedAt.Local().Format(dateLayout)))
	}
	if t.Text != "" {
		lines = append(lines, "", lipgloss.NewStyle().Width(inner).Render(t.Text))
	}
	return styleBox.Width(w - 2).Render(strings.Join(lines, "\n"))
}
