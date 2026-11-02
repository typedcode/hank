package ui

import (
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hoffmannma/hank/internal/store"
)

// focus positions in the task form, in tab order
const (
	focusTitle = iota
	focusText
	focusDue
	focusProject
	focusCancel
	focusSave
	focusCount
)

const formMaxWidth = 72

// batchPlaceholder stands for the list item in the text of a batch task.
const batchPlaceholder = "{$}"

var (
	batchList = regexp.MustCompile(`\{\s*"[^"]+"(?:\s*,\s*"[^"]+")*\s*\}`)
	batchItem = regexp.MustCompile(`"([^"]+)"`)
)

// expandBatch returns one task per item of the first {"a", "b"} list in the
// title, or just t if there is no list.
func expandBatch(t store.Task) []store.Task {
	loc := batchList.FindStringIndex(t.Title)
	if loc == nil {
		return []store.Task{t}
	}
	var tasks []store.Task
	for _, match := range batchItem.FindAllStringSubmatch(t.Title[loc[0]:loc[1]], -1) {
		item := match[1]
		c := t
		c.Title = strings.TrimSpace(t.Title[:loc[0]] + item + t.Title[loc[1]:])
		c.Text = strings.ReplaceAll(t.Text, batchPlaceholder, item)
		tasks = append(tasks, c)
	}
	return tasks
}

// taskForm is the overlay to create (editID == 0) or edit a task.
type taskForm struct {
	editID      int64
	title       textinput.Model
	text        textarea.Model
	due         textinput.Model
	projectID   int64
	projectName string
	focus       int
	err         string

	picking bool
	picker  projectPicker
}

func newInput(placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	return in
}

func (m *Model) openForm(t store.Task) tea.Cmd {
	f := taskForm{
		editID:      t.ID,
		title:       newInput(""),
		text:        textarea.New(),
		due:         newInput("yyyy-mm-dd"),
		projectID:   t.ProjectID,
		projectName: t.ProjectName,
	}
	f.text.Prompt = ""
	f.text.ShowLineNumbers = false
	f.text.CharLimit = 0
	f.text.FocusedStyle.CursorLine = lipgloss.NewStyle()
	f.text.SetHeight(5)
	f.due.CharLimit = len(dateLayout)
	f.resize(m.width)

	f.title.SetValue(t.Title)
	f.text.SetValue(t.Text)
	f.due.SetValue(t.Due)
	m.form = f
	m.mode = modeForm
	return m.form.setFocus(focusTitle)
}

func (f *taskForm) resize(screen int) {
	w := boxInner(boxWidth(screen, formMaxWidth)) - styleField.GetHorizontalFrameSize()
	f.title.Width = w - 1
	f.due.Width = w - 1
	f.text.SetWidth(w)
}

func (f *taskForm) setFocus(i int) tea.Cmd {
	f.focus = (i + focusCount) % focusCount
	f.title.Blur()
	f.text.Blur()
	f.due.Blur()
	switch f.focus {
	case focusTitle:
		return f.title.Focus()
	case focusText:
		return f.text.Focus()
	case focusDue:
		return f.due.Focus()
	}
	return nil
}

func (f *taskForm) updateInputs(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case f.picking:
		f.picker.input, cmd = f.picker.input.Update(msg)
	case f.focus == focusTitle:
		f.title, cmd = f.title.Update(msg)
	case f.focus == focusText:
		f.text, cmd = f.text.Update(msg)
	case f.focus == focusDue:
		f.due, cmd = f.due.Update(msg)
	}
	return cmd
}

func (m *Model) updateForm(msg tea.KeyMsg) tea.Cmd {
	f := &m.form
	if f.picking {
		return m.updatePicker(msg)
	}
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return nil
	case "tab":
		return f.setFocus(f.focus + 1)
	case "shift+tab":
		return f.setFocus(f.focus - 1)
	case "enter":
		switch f.focus {
		case focusTitle:
			m.saveForm()
			return nil
		case focusDue:
			return f.setFocus(f.focus + 1)
		case focusProject:
			return m.openPicker()
		case focusCancel:
			m.mode = modeList
			return nil
		case focusSave:
			m.saveForm()
			return nil
		}
	}
	return f.updateInputs(msg)
}

func (m *Model) saveForm() {
	f := &m.form
	t := store.Task{
		ID:        f.editID,
		Title:     strings.TrimSpace(f.title.Value()),
		Text:      strings.TrimRight(f.text.Value(), "\n "),
		Due:       strings.TrimSpace(f.due.Value()),
		ProjectID: f.projectID,
	}
	if t.Title == "" {
		f.err = "Title is required."
		return
	}
	if t.Due != "" {
		if _, err := time.Parse(dateLayout, t.Due); err != nil {
			f.err = "Due date must be a valid date in the format yyyy-mm-dd."
			return
		}
	}
	var err error
	if t.ID == 0 {
		tasks := expandBatch(t)
		for _, task := range tasks {
			if task.Title == "" {
				f.err = "Title is required."
				return
			}
		}
		var ids []int64
		if ids, err = m.store.CreateTasks(tasks); err == nil {
			t.ID = ids[0]
		}
	} else {
		err = m.store.UpdateTask(t)
	}
	if err != nil {
		f.err = err.Error()
		return
	}
	m.mode = modeList
	m.reload(t.ID)
}

func (m *Model) openPicker() tea.Cmd {
	projects, err := m.store.ActiveProjects()
	if err != nil {
		m.form.err = err.Error()
		return nil
	}
	none := pickOption{label: "(no project)", id: optNoProject, style: styleMuted}
	m.form.picker = newProjectPicker("Choose project", []pickOption{none}, projects, true, m.form.projectID)
	m.form.picking = true
	return nil
}

func (m *Model) updatePicker(msg tea.KeyMsg) tea.Cmd {
	f := &m.form
	choice, closed, cmd := f.picker.update(msg, m.store)
	if choice != nil {
		f.projectID, f.projectName = choice.id, choice.label
		if choice.id == optNoProject {
			f.projectName = ""
		}
	}
	if choice != nil || closed {
		f.picking = false
	}
	return cmd
}

func (f *taskForm) view(screen int) string {
	w := boxWidth(screen, formMaxWidth)
	fieldW := boxInner(w) - styleField.GetHorizontalFrameSize()

	label := func(i int, text string) string {
		if f.focus == i {
			return styleFocused.Render("› " + text)
		}
		return styleLabel.Render("  " + text)
	}
	field := func(i int, content string) string {
		s := styleField
		if f.focus == i {
			s = styleFieldFocused
		}
		return s.Width(fieldW).Render(content)
	}
	button := func(i int, text string) string {
		if f.focus == i {
			return styleButtonActive.Render(text)
		}
		return styleButton.Render(text)
	}

	project := f.projectName
	if project == "" {
		project = styleMuted.Render("(no project)")
	}
	if f.focus == focusProject {
		project += styleMuted.Render("  · enter to choose")
	}

	heading := "New task"
	if f.editID != 0 {
		heading = "Edit task"
	}
	parts := []string{
		styleTitle.Render(heading), "",
		label(focusTitle, "Title"), field(focusTitle, f.title.View()),
		label(focusText, "Text"), field(focusText, f.text.View()),
		label(focusDue, "Due date (yyyy-mm-dd)"), field(focusDue, f.due.View()),
		label(focusProject, "Project"), field(focusProject, project),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, button(focusCancel, "Cancel"), " ", button(focusSave, "Save")),
	}
	if f.err != "" {
		parts = append(parts, "", styleError.Render(f.err))
	}
	return styleBox.Width(w - 2).Render(strings.Join(parts, "\n"))
}
