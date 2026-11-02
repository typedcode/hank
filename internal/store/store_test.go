package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sub", "hank.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// must panics on error, which fails the running test.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func titles(tasks []Task) []string {
	var out []string
	for _, t := range tasks {
		out = append(out, t.Title)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReopenExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hank.db")
	s := must(Open(path))
	must(s.CreateTask(Task{Title: "a"}))
	s.Close()

	s = must(Open(path))
	defer s.Close()
	open, _, err := s.ListTasks()
	if err != nil || len(open) != 1 {
		t.Fatalf("got %v, %v", open, err)
	}
}

func TestOrderingAndMove(t *testing.T) {
	s := openTest(t)
	c := must(s.CreateTask(Task{Title: "c"}))
	must(s.CreateTask(Task{Title: "b"}))
	a := must(s.CreateTask(Task{Title: "a"}))

	open, _, _ := s.ListTasks()
	if got := titles(open); !equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("newest task not on top: %v", got)
	}

	b := open[1].ID
	s.SwapPositions(c, b)
	open, _, _ = s.ListTasks()
	if got := titles(open); !equal(got, []string{"a", "c", "b"}) {
		t.Fatalf("after swap: %v", got)
	}
	s.ToggleFinished(a, time.Now())
	s.SwapPositions(a, c) // finished tasks are not reordered
	open, _, _ = s.ListTasks()
	if got := titles(open); !equal(got, []string{"c", "b"}) {
		t.Fatalf("finished task was swapped: %v", got)
	}
	s.ToggleFinished(a, time.Now())
	s.SwapPositions(a, a)
	open, _, _ = s.ListTasks()
	if got := titles(open); !equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("after self swap: %v", got)
	}
}

func TestStartAndFinish(t *testing.T) {
	s := openTest(t)
	a := must(s.CreateTask(Task{Title: "a"}))
	b := must(s.CreateTask(Task{Title: "b"}))
	c := must(s.CreateTask(Task{Title: "c"}))

	s.ToggleStarted(a)
	if got := must(s.GetTask(a)); got.State != Started {
		t.Fatalf("expected started, got %v", got.State)
	}
	s.ToggleStarted(a)
	if got := must(s.GetTask(a)); got.State != NotStarted {
		t.Fatalf("expected not started, got %v", got.State)
	}

	now := time.Now()
	s.ToggleFinished(a, now)
	s.ToggleFinished(b, now.Add(time.Minute))
	open, finished, _ := s.ListTasks()
	if got := titles(open); !equal(got, []string{"c"}) {
		t.Fatalf("open: %v", got)
	}
	if got := titles(finished); !equal(got, []string{"b", "a"}) {
		t.Fatalf("finished not latest first: %v", got)
	}
	if finished[1].FinishedAt.UnixNano() != now.UnixNano() {
		t.Fatalf("finish date not stored: %v", finished[1].FinishedAt)
	}

	// s has no effect on finished tasks
	s.ToggleStarted(a)
	if got := must(s.GetTask(a)); got.State != Finished {
		t.Fatalf("finished task changed by start toggle: %v", got.State)
	}

	// reopening clears the finish date and appends to the open tasks
	s.ToggleFinished(a, now)
	got := must(s.GetTask(a))
	if got.State != NotStarted || !got.FinishedAt.IsZero() {
		t.Fatalf("reopen: %+v", got)
	}
	open, _, _ = s.ListTasks()
	if titles := titles(open); !equal(titles, []string{"c", "a"}) {
		t.Fatalf("reopened task not at bottom: %v", titles)
	}
	_ = c
}

func TestProjects(t *testing.T) {
	s := openTest(t)
	work := must(s.CreateProject("Work"))
	if _, err := s.CreateProject("work"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if _, err := s.CreateProject("  "); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("expected empty name error, got %v", err)
	}
	home := must(s.CreateProject("Home"))
	if err := s.RenameProject(home, "Work"); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("expected duplicate error on rename, got %v", err)
	}

	task := must(s.CreateTask(Task{Title: "report", ProjectID: work}))
	must(s.CreateTask(Task{Title: "loose"}))
	if n := must(s.OpenTaskCount(work)); n != 1 {
		t.Fatalf("open count: %d", n)
	}

	s.SetProjectActive(work, false)
	open, _, _ := s.ListTasks()
	if got := titles(open); !equal(got, []string{"loose"}) {
		t.Fatalf("inactive project's task visible: %v", got)
	}
	if _, err := s.CreateTask(Task{Title: "x", ProjectID: work}); !errors.Is(err, ErrProjectInactive) {
		t.Fatalf("expected inactive error, got %v", err)
	}
	if active := must(s.ActiveProjects()); len(active) != 1 || active[0].Name != "Home" {
		t.Fatalf("active projects: %v", active)
	}

	s.SetProjectActive(work, true)
	open, _, _ = s.ListTasks()
	if got := titles(open); !equal(got, []string{"loose", "report"}) {
		t.Fatalf("reactivated task not back in place: %v", got)
	}
	if open[1].ProjectName != "Work" {
		t.Fatalf("project name not joined: %+v", open[0])
	}

	s.ToggleFinished(task, time.Now())
	must(s.CreateTask(Task{Title: "another", ProjectID: work}))
	s.DeleteProject(work)
	open, finished, _ := s.ListTasks()
	if len(finished) != 0 || !equal(titles(open), []string{"loose"}) {
		t.Fatalf("project tasks not deleted: %v %v", titles(open), titles(finished))
	}
	if all := must(s.ListProjects()); len(all) != 1 {
		t.Fatalf("projects after delete: %v", all)
	}
}

func TestFilterAndPaging(t *testing.T) {
	s := openTest(t)
	work := must(s.CreateProject("Work"))
	must(s.CreateTask(Task{Title: "Äpfel kaufen"}))
	must(s.CreateTask(Task{Title: "report", Text: "for the ÄRGER department", ProjectID: work}))
	must(s.CreateTask(Task{Title: "100% done"}))
	start := time.Now()
	for i := range 5 {
		id := must(s.CreateTask(Task{Title: fmt.Sprintf("old %d", i), ProjectID: work}))
		s.ToggleFinished(id, start.Add(time.Duration(i)*time.Minute))
	}

	open := must(s.ListOpen(Filter{Search: "äp", ProjectID: AllProjects}))
	if got := titles(open); !equal(got, []string{"Äpfel kaufen"}) {
		t.Fatalf("unicode case-insensitive title search: %v", got)
	}
	open = must(s.ListOpen(Filter{Search: "ärger", ProjectID: AllProjects}))
	if got := titles(open); !equal(got, []string{"report"}) {
		t.Fatalf("text search: %v", got)
	}
	open = must(s.ListOpen(Filter{Search: "%", ProjectID: AllProjects}))
	if got := titles(open); !equal(got, []string{"100% done"}) {
		t.Fatalf("search must treat %% literally: %v", got)
	}
	open = must(s.ListOpen(Filter{ProjectID: 0}))
	if got := titles(open); !equal(got, []string{"100% done", "Äpfel kaufen"}) {
		t.Fatalf("no project filter: %v", got)
	}
	open = must(s.ListOpen(Filter{ProjectID: work}))
	if got := titles(open); !equal(got, []string{"report"}) {
		t.Fatalf("project filter: %v", got)
	}

	page, more, err := s.ListFinished(NoFilter, 0, 2)
	if err != nil || !more || !equal(titles(page), []string{"old 4", "old 3"}) {
		t.Fatalf("first page: %v %v %v", titles(page), more, err)
	}
	page, more, _ = s.ListFinished(NoFilter, 4, 2)
	if more || !equal(titles(page), []string{"old 0"}) {
		t.Fatalf("last page: %v %v", titles(page), more)
	}
	page, more, _ = s.ListFinished(Filter{Search: "OLD 1", ProjectID: work}, 0, 2)
	if more || !equal(titles(page), []string{"old 1"}) {
		t.Fatalf("search in finished: %v %v", titles(page), more)
	}
	page, more, _ = s.ListFinished(NoFilter, 0, 5)
	if more || len(page) != 5 {
		t.Fatalf("exact page: %d %v", len(page), more)
	}
}
