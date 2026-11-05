package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hoffmannma/hank/internal/config"
	"github.com/hoffmannma/hank/internal/store"
)

func newTestModel(t *testing.T) (*Model, *store.Store) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := store.Open(filepath.Join(t.TempDir(), "hank.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := New(s, config.Config{FinishedPageSize: 50})
	m.now = func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.Local) }
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return m, s
}

var specialKeys = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown, "space": tea.KeySpace,
}

// press sends keys; each argument is a special key name or a single rune.
func press(m *Model, keys ...string) {
	for _, k := range keys {
		if kt, ok := specialKeys[k]; ok {
			m.Update(tea.KeyMsg{Type: kt})
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

// typeText types s rune by rune; "\n" becomes enter.
func typeText(m *Model, s string) {
	for _, r := range s {
		if r == '\n' {
			press(m, "enter")
		} else {
			press(m, string(r))
		}
	}
}

func openTitles(t *testing.T, s *store.Store) []string {
	t.Helper()
	open, _, err := s.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, task := range open {
		out = append(out, task.Title)
	}
	return out
}

// createTask creates a task by pressing enter in the title field.
func createTask(m *Model, title string) {
	press(m, "c")
	typeText(m, title)
	press(m, "enter")
}

func screen(m *Model) string { return ansi.Strip(m.View()) }

func TestCreateTaskThroughForm(t *testing.T) {
	m, s := newTestModel(t)
	press(m, "c")
	typeText(m, "Write spec")
	press(m, "tab")
	typeText(m, "line one\nline two")
	press(m, "tab")
	typeText(m, "2026-10-01")
	press(m, "tab", "tab", "tab") // project → cancel → save
	if m.form.focus != focusSave {
		t.Fatalf("expected save button focused, got %d", m.form.focus)
	}
	press(m, "enter")

	if m.mode != modeList {
		t.Fatalf("form still open: %q", m.form.err)
	}
	open, _, _ := s.ListTasks()
	if len(open) != 1 {
		t.Fatalf("expected one task, got %v", open)
	}
	got := open[0]
	if got.Title != "Write spec" || got.Text != "line one\nline two" || got.Due != "2026-10-01" {
		t.Fatalf("unexpected task %+v", got)
	}
}

func TestFormValidationAndCancel(t *testing.T) {
	m, s := newTestModel(t)
	press(m, "c", "shift+tab", "enter") // save with empty title
	if m.mode != modeForm || !strings.Contains(m.form.err, "Title") {
		t.Fatalf("expected title error, mode %v err %q", m.mode, m.form.err)
	}

	press(m, "tab") // wrap around to title
	typeText(m, "x")
	press(m, "tab", "tab")
	typeText(m, "2026-13-01")
	press(m, "tab", "tab", "tab", "enter") // due → project → cancel → save
	if m.mode != modeForm || !strings.Contains(m.form.err, "yyyy-mm-dd") {
		t.Fatalf("expected date error, mode %v err %q focus %d", m.mode, m.form.err, m.form.focus)
	}

	press(m, "esc")
	if m.mode != modeList || len(openTitles(t, s)) != 0 {
		t.Fatal("cancel did not discard the task")
	}

	// cancel button
	press(m, "c")
	typeText(m, "y")
	press(m, "shift+tab", "shift+tab", "enter")
	if m.mode != modeList || len(openTitles(t, s)) != 0 {
		t.Fatal("cancel button did not discard the task")
	}
}

func TestProjectPickerCreatesProject(t *testing.T) {
	m, s := newTestModel(t)
	s.CreateProject("Old")
	id, _ := s.CreateProject("Inactive")
	s.SetProjectActive(id, false)

	press(m, "c")
	typeText(m, "task")
	press(m, "tab", "tab", "tab", "enter")
	if !m.form.picking || len(m.form.picker.options) != 3 { // none, Old, new
		t.Fatalf("picker should show only active projects: %+v", m.form.picker.options)
	}
	press(m, "down", "down", "enter") // "+ New project…"
	typeText(m, "old")
	press(m, "enter")
	if !strings.Contains(m.form.picker.err, "already exists") {
		t.Fatalf("expected duplicate error, got %q", m.form.picker.err)
	}
	for range 3 {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeText(m, "Work")
	press(m, "enter")
	if m.form.picking || m.form.projectName != "Work" {
		t.Fatalf("project not selected: %+v", m.form)
	}
	press(m, "tab", "tab", "enter")

	open, _, _ := s.ListTasks()
	if len(open) != 1 || open[0].ProjectName != "Work" {
		t.Fatalf("task not assigned: %+v", open)
	}
}

func TestListActions(t *testing.T) {
	m, s := newTestModel(t)
	createTask(m, "c")
	createTask(m, "b")
	createTask(m, "a")
	if got := openTitles(t, s); strings.Join(got, "") != "abc" {
		t.Fatalf("newest task not on top: %v", got)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor should be on the new task, got %d", m.cursor)
	}

	press(m, "down", "down", "+", "+", "+")
	if got := openTitles(t, s); strings.Join(got, "") != "cab" || m.cursor != 0 {
		t.Fatalf("after moving up: %v cursor %d", got, m.cursor)
	}

	press(m, "space")
	if task, _ := m.selected(); task.State != store.Started {
		t.Fatalf("not started: %+v", task)
	}
	press(m, "f")
	if task, _ := m.selected(); len(m.finished) != 1 || task.Title != "a" {
		t.Fatalf("finish: finished=%d selected=%q", len(m.finished), task.Title)
	}
	press(m, "down", "down") // into the finished section
	if !strings.Contains(screen(m), "2026-09-24") {
		t.Fatal("finish date not displayed")
	}
	press(m, "-", "+", "space") // no effect on finished tasks
	if task, _ := m.selected(); task.State != store.Finished || strings.Join(openTitles(t, s), "") != "ab" {
		t.Fatalf("finished task was changed: %+v", task)
	}

	press(m, "f") // reopen → bottom of open tasks
	if got := openTitles(t, s); strings.Join(got, "") != "abc" || m.cursor != 2 {
		t.Fatalf("reopen: %v cursor %d", got, m.cursor)
	}

	press(m, "d", "n")
	if len(openTitles(t, s)) != 3 {
		t.Fatal("deleted without confirmation")
	}
	press(m, "d")
	if !strings.Contains(screen(m), `Delete task "c"?`) {
		t.Fatal("no confirmation shown")
	}
	press(m, "y")
	if got := openTitles(t, s); strings.Join(got, "") != "ab" {
		t.Fatalf("after delete: %v", got)
	}
}

func TestEditAndDetail(t *testing.T) {
	m, s := newTestModel(t)
	createTask(m, "old")
	press(m, "e")
	for range 3 {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeText(m, "new")
	press(m, "tab")
	typeText(m, "some details")
	press(m, "tab")
	typeText(m, "2026-09-01")
	press(m, "tab", "tab", "tab", "enter")
	if got := openTitles(t, s); len(got) != 1 || got[0] != "new" {
		t.Fatalf("edit failed: %v (%q)", got, m.form.err)
	}

	press(m, "enter")
	view := screen(m)
	if m.mode != modeDetail || !strings.Contains(view, "some details") || !strings.Contains(view, "overdue") {
		t.Fatalf("detail view wrong:\n%s", view)
	}
	press(m, "esc")
	if m.mode != modeList {
		t.Fatal("detail not closed")
	}
}

func TestProjectsOverlay(t *testing.T) {
	m, s := newTestModel(t)
	press(m, "p", "c")
	typeText(m, "Work")
	press(m, "enter")
	press(m, "c")
	typeText(m, "work")
	press(m, "enter")
	if !strings.Contains(m.projects.err, "already exists") {
		t.Fatalf("expected duplicate error, got %q", m.projects.err)
	}
	press(m, "esc", "esc")

	work := m.projects.items
	if len(work) != 1 {
		t.Fatalf("projects: %+v", work)
	}
	s.CreateTask(store.Task{Title: "report", ProjectID: work[0].ID})
	createTask(m, "loose")

	// deactivate with open tasks: confirmation mentions them
	press(m, "p", "a")
	if !strings.Contains(m.confirm.message, "still has 1 open task") {
		t.Fatalf("confirmation: %q", m.confirm.message)
	}
	press(m, "y")
	if m.mode != modeProjects || strings.Join(openTitles(t, s), ",") != "loose" {
		t.Fatalf("deactivate: mode %v tasks %v", m.mode, openTitles(t, s))
	}
	press(m, "a", "y") // activate again
	if strings.Join(openTitles(t, s), ",") != "loose,report" {
		t.Fatalf("reactivate: %v", openTitles(t, s))
	}

	// rename
	press(m, "e")
	typeText(m, "!")
	press(m, "enter")
	if m.projects.items[0].Name != "Work!" {
		t.Fatalf("rename: %+v", m.projects.items)
	}

	// delete with open tasks needs two confirmations
	press(m, "d", "y")
	if m.mode != modeConfirm || !strings.Contains(m.confirm.message, "removed as well") {
		t.Fatalf("second confirmation missing: %v %q", m.mode, m.confirm.message)
	}
	press(m, "y")
	if len(m.projects.items) != 0 || strings.Join(openTitles(t, s), ",") != "loose" {
		t.Fatalf("delete: %+v %v", m.projects.items, openTitles(t, s))
	}
	press(m, "q")
	if m.mode != modeList {
		t.Fatal("projects overlay not closed")
	}
}

func TestViewSizes(t *testing.T) {
	m, _ := newTestModel(t)
	createTask(m, strings.Repeat("long title ", 20))
	for _, size := range [][2]int{{20, 5}, {80, 24}, {200, 60}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, keys := range [][]string{nil, {"enter"}, {"esc", "c"}, {"esc", "p"}, {"esc", "s"}, {"esc", "d"}} {
			press(m, keys...)
			lines := strings.Split(m.View(), "\n")
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > max(size[0], 20)+60 {
					t.Fatalf("line too wide (%d) at %v after %v:\n%s", w, size, keys, ansi.Strip(m.View()))
				}
			}
		}
		press(m, "n", "esc")
	}
}

func visibleTitles(m *Model) string {
	var out []string
	for _, t := range m.tasks() {
		out = append(out, t.Title)
	}
	return strings.Join(out, ",")
}

func TestSearch(t *testing.T) {
	m, s := newTestModel(t)
	createTask(m, "Done")
	s.CreateTask(store.Task{Title: "Call", Text: "about the MILK delivery"})
	createTask(m, "Write report")
	createTask(m, "Buy milk")
	m.cursor = 3
	press(m, "f")

	press(m, "/")
	if m.mode != modeSearch {
		t.Fatal("search not focused")
	}
	typeText(m, "milk") // j/k are typed, not used for navigation
	if got := visibleTitles(m); got != "Buy milk,Call" {
		t.Fatalf("search by title and text: %q", got)
	}
	press(m, "enter")
	if m.mode != modeList || visibleTitles(m) != "Buy milk,Call" {
		t.Fatalf("enter should keep the search: %v %q", m.mode, visibleTitles(m))
	}
	if !strings.Contains(screen(m), "milk") {
		t.Fatal("search text not shown above the table")
	}

	press(m, "esc")
	if got := visibleTitles(m); got != "Buy milk,Write report,Call,Done" {
		t.Fatalf("esc in list should clear the search: %q", got)
	}

	press(m, "/")
	typeText(m, "zzz")
	if !strings.Contains(screen(m), "No matching tasks") {
		t.Fatal("empty result not explained")
	}
	press(m, "esc")
	if m.mode != modeList || m.search.Value() != "" || visibleTitles(m) != "Buy milk,Write report,Call,Done" {
		t.Fatalf("esc in search should clear it: %q", visibleTitles(m))
	}
}

func TestProjectFilter(t *testing.T) {
	m, s := newTestModel(t)
	work, _ := s.CreateProject("Work")
	home, _ := s.CreateProject("Home")
	s.CreateTask(store.Task{Title: "w2", ProjectID: work})
	s.CreateTask(store.Task{Title: "loose"})
	s.CreateTask(store.Task{Title: "h1", ProjectID: home})
	s.CreateTask(store.Task{Title: "w1", ProjectID: work})
	m.reload(0)

	press(m, "F")
	if m.mode != modeFilter {
		t.Fatal("filter picker not open")
	}
	opts := m.filterPicker.options
	if len(opts) != 4 || opts[len(opts)-1].id == optNewProject {
		t.Fatalf("filter must not offer creating a project: %+v", opts)
	}
	press(m, "down", "down", "down", "enter") // all, none, Home, Work
	if got := visibleTitles(m); got != "w1,w2" {
		t.Fatalf("filter Work: %q", got)
	}

	// moving swaps with the visible neighbour
	m.cursor = 1
	press(m, "+")
	if got := strings.Join(openTitles(t, s), ","); got != "w2,h1,loose,w1" {
		t.Fatalf("move in filtered list: %q", got)
	}
	if task, _ := m.selected(); task.Title != "w2" {
		t.Fatalf("cursor should follow the moved task: %+v", task)
	}

	// tab in the search field jumps to the project filter, keeping the search
	press(m, "/")
	typeText(m, "w")
	press(m, "tab")
	if m.mode != modeFilter || m.search.Value() != "w" {
		t.Fatalf("tab should open the project filter: mode %v search %q", m.mode, m.search.Value())
	}
	press(m, "esc", "/")
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	press(m, "enter")
	if m.search.Value() != "" {
		t.Fatalf("search not cleared: %q", m.search.Value())
	}

	press(m, "F", "up", "up", "enter") // (no project)
	if got := visibleTitles(m); got != "loose" {
		t.Fatalf("filter no project: %q", got)
	}

	// deleting the filtered project resets the filter
	press(m, "F", "down", "enter") // Home
	press(m, "p", "d", "y", "y", "esc")
	if m.filter.projectID != optAllProjects || visibleTitles(m) != "w2,loose,w1" {
		t.Fatalf("filter not reset after delete: %+v %q", m.filter, visibleTitles(m))
	}
}

func TestFinishedPaging(t *testing.T) {
	m, s := newTestModel(t)
	m.pageSize = 10
	start := time.Now()
	for i := range 25 {
		id, _ := s.CreateTask(store.Task{Title: fmt.Sprintf("done %02d", i)})
		s.ToggleFinished(id, start.Add(time.Duration(i)*time.Minute))
	}
	s.CreateTask(store.Task{Title: "open"})
	m.refilter()
	if len(m.finished) != 10 || !m.finishedMore || m.finished[0].Title != "done 24" {
		t.Fatalf("first batch: %d more=%v", len(m.finished), m.finishedMore)
	}
	if !strings.Contains(screen(m), "more finished tasks") {
		t.Fatal("no hint that more tasks exist")
	}

	for range 6 { // cursor 6 is within 5 of the last loaded task (index 10)
		press(m, "down")
	}
	if len(m.finished) != 20 {
		t.Fatalf("next batch not loaded: %d", len(m.finished))
	}
	for range 20 {
		press(m, "down")
	}
	if len(m.finished) != 25 || m.finishedMore || m.cursor != 25 {
		t.Fatalf("all loaded: %d more=%v cursor=%d", len(m.finished), m.finishedMore, m.cursor)
	}
	if strings.Contains(screen(m), "more finished tasks") {
		t.Fatal("hint shown although everything is loaded")
	}

	// reopening keeps the loaded amount
	press(m, "up", "f")
	if len(m.finished) != 24 || len(m.open) != 2 {
		t.Fatalf("after reopen: finished=%d open=%d", len(m.finished), len(m.open))
	}

	// search covers finished tasks that are not loaded yet
	m.refilter()
	press(m, "/")
	typeText(m, "done 01")
	if got := visibleTitles(m); got != "done 01" {
		t.Fatalf("search in all finished tasks: %q", got)
	}
	press(m, "esc")
	if len(m.finished) != 10 {
		t.Fatalf("clearing the search should start with the first batch: %d", len(m.finished))
	}
}

func TestSelectionAfterFinish(t *testing.T) {
	m, s := newTestModel(t)
	for _, title := range []string{"d", "c", "b", "a"} {
		createTask(m, title) // newest on top: a, b, c, d
	}
	selectedTitle := func() string {
		task, _ := m.selected()
		return task.Title
	}

	press(m, "down", "f") // finish b → c selected
	if got := selectedTitle(); got != "c" {
		t.Fatalf("after finishing b: selected %q", got)
	}
	press(m, "down", "f") // finish d (last open) → c selected
	if got := selectedTitle(); got != "c" || m.cursor >= len(m.open) {
		t.Fatalf("after finishing the last open task: selected %q", got)
	}

	// reopening keeps the task selected
	m.cursor = len(m.open) // first finished task: d
	press(m, "f")
	if got := selectedTitle(); got != "d" {
		t.Fatalf("after reopening d: selected %q", got)
	}
	if task, _ := m.selected(); task.State == store.Finished {
		t.Fatal("reopened task still finished")
	}

	// finishing the only open task leaves nothing else to select
	for len(m.open) > 1 {
		m.cursor = 0
		press(m, "f")
	}
	m.cursor = 0
	press(m, "f")
	if len(m.open) != 0 {
		t.Fatalf("open tasks left: %d", len(openTitles(t, s)))
	}
}

func TestBatchCreate(t *testing.T) {
	m, s := newTestModel(t)
	press(m, "c")
	typeText(m, `update libs for {"proj1", "proj2","proj3"}`)
	press(m, "tab")
	typeText(m, "tests of {$}\nand {$} again")
	press(m, "tab")
	typeText(m, "2026-10-01")
	press(m, "tab", "tab", "tab", "enter")

	if m.mode != modeList {
		t.Fatalf("form still open: %q", m.form.err)
	}
	open, _, _ := s.ListTasks()
	if len(open) != 3 {
		t.Fatalf("expected three tasks, got %v", open)
	}
	for i, name := range []string{"proj1", "proj2", "proj3"} {
		got := open[i]
		if got.Title != "update libs for "+name || got.Text != "tests of "+name+"\nand "+name+" again" || got.Due != "2026-10-01" {
			t.Fatalf("unexpected task %d: %+v", i, got)
		}
	}
	if sel, ok := m.selected(); !ok || sel.ID != open[0].ID {
		t.Fatalf("first task not selected: %+v", sel)
	}
}

func TestExpandBatch(t *testing.T) {
	cases := []struct {
		title string
		want  []string
	}{
		{`plain`, []string{"plain"}},
		{`{"a"}`, []string{"a"}},
		{`{"a", "b"} done`, []string{"a done", "b done"}},
		{`x { "a" , "b" } y {"c", "d"}`, []string{`x a y {"c", "d"}`, `x b y {"c", "d"}`}},
		{`not a list {a, b}`, []string{`not a list {a, b}`}},
		{`empty {""}`, []string{`empty {""}`}},
		{`{"a", b}`, []string{`{"a", b}`}},
	}
	for _, c := range cases {
		var got []string
		for _, task := range expandBatch(store.Task{Title: c.title}) {
			got = append(got, task.Title)
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%q: got %q, want %q", c.title, got, c.want)
		}
	}
}

func TestBatchOnlyOnCreate(t *testing.T) {
	m, s := newTestModel(t)
	createTask(m, "plain")
	press(m, "e")
	typeText(m, ` {"a", "b"}`)
	press(m, "tab")
	typeText(m, "{$}")
	press(m, "shift+tab", "enter")

	open, _, _ := s.ListTasks()
	if len(open) != 1 || open[0].Title != `plain {"a", "b"}` || open[0].Text != "{$}" {
		t.Fatalf("edit expanded the list: %+v", open)
	}

	press(m, "c")
	typeText(m, `{" "}`)
	press(m, "enter")
	if m.mode != modeForm || m.form.err == "" {
		t.Fatalf("blank item accepted, mode %v", m.mode)
	}
}

func TestSettings(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "s")
	if m.mode != modeSettings || !strings.Contains(screen(m), "Settings") {
		t.Fatalf("settings not open:\n%s", screen(m))
	}

	// cancel keeps everything
	press(m, "tab")
	typeText(m, "7")
	press(m, "esc")
	if m.mode != modeList || m.pageSize != 50 {
		t.Fatalf("cancel: mode %d, page size %d", m.mode, m.pageSize)
	}
	if cfg, err := config.Load(); err != nil || cfg != (config.Config{}) {
		t.Fatalf("cancel wrote config: %+v, %v", cfg, err)
	}

	// the current values are shown, invalid ones are rejected
	press(m, "s", "tab")
	if got := m.settings.pageSize.Value(); got != "50" {
		t.Fatalf("page size field: %q", got)
	}
	typeText(m, "x\n")
	if m.mode != modeSettings || !strings.Contains(screen(m), "positive number") {
		t.Fatalf("invalid page size accepted:\n%s", screen(m))
	}

	// typing "s" and space in a field does not trigger list actions
	dbPath := filepath.Join(t.TempDir(), "sub", "other s.db")
	press(m, "esc", "s")
	typeText(m, dbPath)
	press(m, "tab")
	m.settings.pageSize.SetValue("7")
	press(m, "tab", "tab", "enter")
	if m.mode != modeList {
		t.Fatalf("not saved:\n%s", screen(m))
	}
	if m.pageSize != 7 {
		t.Fatalf("page size not applied: %d", m.pageSize)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := (config.Config{DBPath: dbPath, FinishedPageSize: 7}); cfg != want {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}

	// emptying a field goes back to the default
	press(m, "s", "tab")
	m.settings.pageSize.SetValue("")
	press(m, "enter")
	if m.pageSize != config.DefaultFinishedPageSize {
		t.Fatalf("default page size not used: %d", m.pageSize)
	}
	if cfg, _ := config.Load(); cfg.FinishedPageSize != 0 {
		t.Fatalf("default page size written: %+v", cfg)
	}
}

func TestSettingsRejectsBrokenDBPath(t *testing.T) {
	m, _ := newTestModel(t)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	press(m, "s")
	typeText(m, filepath.Join(file, "hank.db")+"\n")
	if m.mode != modeSettings || !strings.Contains(screen(m), "Cannot open database") {
		t.Fatalf("broken path accepted:\n%s", screen(m))
	}
	if cfg, _ := config.Load(); cfg.DBPath != "" {
		t.Fatalf("broken path written: %+v", cfg)
	}
}
