package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hoffmannma/hank/internal/store"
)

// special option ids besides real project ids
const (
	optNoProject   int64 = 0
	optAllProjects       = store.AllProjects
	optNewProject  int64 = -2
)

type pickOption struct {
	label string
	id    int64
	style lipgloss.Style
}

// projectPicker is a small overlay to choose a project. With the create
// option it can also create a new project.
type projectPicker struct {
	title    string
	options  []pickOption
	cursor   int
	creating bool
	input    textinput.Model
	err      string
}

func newProjectPicker(title string, first []pickOption, projects []store.Project, create bool, selected int64) projectPicker {
	p := projectPicker{title: title, options: first, input: newInput("project name")}
	for _, pr := range projects {
		p.options = append(p.options, pickOption{label: pr.Name, id: pr.ID})
	}
	if create {
		p.options = append(p.options, pickOption{label: "+ New project…", id: optNewProject,
			style: lipgloss.NewStyle().Foreground(colorAccent)})
	}
	for i, o := range p.options {
		if o.id == selected {
			p.cursor = i
		}
	}
	return p
}

// update handles a key. It returns the chosen option once one is selected
// (or created), or closed when the picker was left without a choice.
func (p *projectPicker) update(msg tea.KeyMsg, s *store.Store) (choice *pickOption, closed bool, cmd tea.Cmd) {
	if p.creating {
		switch msg.String() {
		case "esc":
			p.creating = false
			p.err = ""
			p.input.Blur()
			return nil, false, nil
		case "enter":
			name := strings.TrimSpace(p.input.Value())
			id, err := s.CreateProject(name)
			if err != nil {
				p.err = err.Error()
				return nil, false, nil
			}
			return &pickOption{label: name, id: id}, false, nil
		}
		p.input, cmd = p.input.Update(msg)
		return nil, false, cmd
	}
	switch msg.String() {
	case "esc":
		return nil, true, nil
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = min(len(p.options)-1, p.cursor+1)
	case "enter":
		o := p.options[p.cursor]
		if o.id == optNewProject {
			p.creating = true
			p.input.SetValue("")
			return nil, false, p.input.Focus()
		}
		return &o, false, nil
	}
	return nil, false, nil
}

func (p *projectPicker) view(screen int) string {
	w := boxWidth(screen, 48)
	inner := boxInner(w)
	parts := []string{styleTitle.Render(p.title), ""}

	if p.creating {
		p.input.Width = inner - styleField.GetHorizontalFrameSize() - 1
		parts = append(parts, styleLabel.Render("New project name"),
			styleFieldFocused.Width(inner-styleField.GetHorizontalFrameSize()).Render(p.input.View()))
		if p.err != "" {
			parts = append(parts, "", styleError.Render(p.err))
		}
		return styleBox.Width(w - 2).Render(strings.Join(parts, "\n"))
	}

	for i, o := range p.options {
		if i == p.cursor {
			parts = append(parts, o.style.Background(colorSelected).Render(fit("› "+o.label, inner)))
		} else {
			parts = append(parts, o.style.Render(fit("  "+o.label, inner)))
		}
	}
	return styleBox.Width(w - 2).Render(strings.Join(parts, "\n"))
}
