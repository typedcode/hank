// Package store persists tasks and projects in a SQLite database.
package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"
)

func init() {
	// SQLite's lower() only handles ASCII; searching needs full Unicode
	// case folding (e.g. "Ä" and "ä").
	sqlite.MustRegisterDeterministicScalarFunction("hank_lower", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case string:
				return strings.ToLower(v), nil
			case []byte:
				return strings.ToLower(string(v)), nil
			}
			return args[0], nil
		})
}

// State is the progress of a task.
type State int

const (
	NotStarted State = iota
	Started
	Finished
)

// Task is a single task. ProjectID is 0 if the task has no project.
type Task struct {
	ID          int64
	Title       string
	Text        string
	Due         string // yyyy-mm-dd or empty
	ProjectID   int64
	ProjectName string
	State       State
	FinishedAt  time.Time // zero unless finished
	Position    int64
}

// Project groups tasks. Tasks of inactive projects are hidden.
type Project struct {
	ID     int64
	Name   string
	Active bool
}

var (
	ErrDuplicateName   = errors.New("a project with this name already exists")
	ErrEmptyName       = errors.New("project name must not be empty")
	ErrProjectInactive = errors.New("tasks can not be assigned to an inactive project")
)

const schema = `
CREATE TABLE projects (
	id     INTEGER PRIMARY KEY,
	name   TEXT NOT NULL UNIQUE COLLATE NOCASE,
	active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE tasks (
	id          INTEGER PRIMARY KEY,
	title       TEXT NOT NULL,
	text        TEXT NOT NULL DEFAULT '',
	due_date    TEXT,
	project_id  INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	state       INTEGER NOT NULL DEFAULT 0,
	finished_at INTEGER,
	position    INTEGER NOT NULL
);
CREATE INDEX tasks_project ON tasks(project_id);
`

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// Open opens (and if needed creates) the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init database %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 1 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
		return err
	}
	return tx.Commit()
}

// visible restricts a task query to tasks without a project or with an
// active project.
const visible = `(t.project_id IS NULL OR p.active = 1)`

const taskColumns = `t.id, t.title, t.text, COALESCE(t.due_date, ''), COALESCE(t.project_id, 0),
	COALESCE(p.name, ''), t.state, t.finished_at, t.position
	FROM tasks t LEFT JOIN projects p ON p.id = t.project_id`

func scanTasks(rows *sql.Rows) ([]Task, error) {
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var finished sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Title, &t.Text, &t.Due, &t.ProjectID,
			&t.ProjectName, &t.State, &finished, &t.Position); err != nil {
			return nil, err
		}
		if finished.Valid {
			t.FinishedAt = time.Unix(0, finished.Int64)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// AllProjects as Filter.ProjectID matches tasks of any project.
const AllProjects int64 = -1

// Filter restricts the listed tasks. Search matches title or text,
// ignoring case. ProjectID is AllProjects, 0 for tasks without a project, or
// a project id.
type Filter struct {
	Search    string
	ProjectID int64
}

// NoFilter lists all visible tasks.
var NoFilter = Filter{ProjectID: AllProjects}

func (f Filter) where() (string, []any) {
	cond, args := visible, []any{}
	switch {
	case f.ProjectID == 0:
		cond += ` AND t.project_id IS NULL`
	case f.ProjectID > 0:
		cond += ` AND t.project_id = ?`
		args = append(args, f.ProjectID)
	}
	if q := strings.ToLower(strings.TrimSpace(f.Search)); q != "" {
		cond += ` AND (instr(hank_lower(t.title), ?) > 0 OR instr(hank_lower(t.text), ?) > 0)`
		args = append(args, q, q)
	}
	return cond, args
}

// ListOpen returns the visible unfinished tasks matching f in manual order.
func (s *Store) ListOpen(f Filter) ([]Task, error) {
	cond, args := f.where()
	rows, err := s.db.Query(`SELECT `+taskColumns+` WHERE `+cond+` AND t.state != ? ORDER BY t.position`,
		append(args, Finished)...)
	if err != nil {
		return nil, err
	}
	return scanTasks(rows)
}

// ListFinished returns up to limit visible finished tasks matching f,
// latest finished first, skipping the first offset. more reports whether
// further tasks exist. A negative limit returns all.
func (s *Store) ListFinished(f Filter, offset, limit int) (tasks []Task, more bool, err error) {
	cond, args := f.where()
	query := limit
	if limit >= 0 {
		query = limit + 1 // one extra row tells whether there are more
	}
	rows, err := s.db.Query(`SELECT `+taskColumns+` WHERE `+cond+` AND t.state = ? ORDER BY t.finished_at DESC LIMIT ? OFFSET ?`,
		append(args, Finished, query, offset)...)
	if err != nil {
		return nil, false, err
	}
	if tasks, err = scanTasks(rows); err != nil {
		return nil, false, err
	}
	if limit >= 0 && len(tasks) > limit {
		return tasks[:limit], true, nil
	}
	return tasks, false, nil
}

// ListTasks returns all visible open and finished tasks without filter.
func (s *Store) ListTasks() (open, finished []Task, err error) {
	if open, err = s.ListOpen(NoFilter); err != nil {
		return nil, nil, err
	}
	finished, _, err = s.ListFinished(NoFilter, 0, -1)
	return open, finished, err
}

// GetTask returns a single task.
func (s *Store) GetTask(id int64) (Task, error) {
	rows, err := s.db.Query(`SELECT `+taskColumns+` WHERE t.id = ?`, id)
	if err != nil {
		return Task{}, err
	}
	tasks, err := scanTasks(rows)
	if err != nil {
		return Task{}, err
	}
	if len(tasks) == 0 {
		return Task{}, sql.ErrNoRows
	}
	return tasks[0], nil
}

func nullable[T comparable](v T) any {
	var zero T
	if v == zero {
		return nil
	}
	return v
}

func (s *Store) checkProject(projectID int64) error {
	if projectID == 0 {
		return nil
	}
	var active bool
	if err := s.db.QueryRow(`SELECT active FROM projects WHERE id = ?`, projectID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrProjectInactive
	}
	return nil
}

func (s *Store) firstPosition() (int64, error) {
	var pos int64
	err := s.db.QueryRow(`SELECT COALESCE(MIN(position), 0) - 1 FROM tasks`).Scan(&pos)
	return pos, err
}

func (s *Store) nextPosition() (int64, error) {
	var pos int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM tasks`).Scan(&pos)
	return pos, err
}

// CreateTask adds a new, not started task at the top of the open tasks.
func (s *Store) CreateTask(t Task) (int64, error) {
	if err := s.checkProject(t.ProjectID); err != nil {
		return 0, err
	}
	pos, err := s.firstPosition()
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO tasks (title, text, due_date, project_id, state, position) VALUES (?, ?, ?, ?, ?, ?)`,
		t.Title, t.Text, nullable(t.Due), nullable(t.ProjectID), NotStarted, pos)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateTasks adds several tasks at once, or none on error. They end up at the
// top of the open tasks in the given order and their IDs are returned.
func (s *Store) CreateTasks(tasks []Task) ([]int64, error) {
	for _, t := range tasks {
		if err := s.checkProject(t.ProjectID); err != nil {
			return nil, err
		}
	}
	first, err := s.firstPosition()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pos := first - int64(len(tasks)-1)
	ids := make([]int64, 0, len(tasks))
	for i, t := range tasks {
		res, err := tx.Exec(`INSERT INTO tasks (title, text, due_date, project_id, state, position) VALUES (?, ?, ?, ?, ?, ?)`,
			t.Title, t.Text, nullable(t.Due), nullable(t.ProjectID), NotStarted, pos+int64(i))
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, tx.Commit()
}

// UpdateTask changes the editable fields of a task.
func (s *Store) UpdateTask(t Task) error {
	var current int64
	if err := s.db.QueryRow(`SELECT COALESCE(project_id, 0) FROM tasks WHERE id = ?`, t.ID).Scan(&current); err != nil {
		return err
	}
	if t.ProjectID != current {
		if err := s.checkProject(t.ProjectID); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE tasks SET title = ?, text = ?, due_date = ?, project_id = ? WHERE id = ?`,
		t.Title, t.Text, nullable(t.Due), nullable(t.ProjectID), t.ID)
	return err
}

func (s *Store) DeleteTask(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

// ToggleStarted switches an open task between not started and started.
func (s *Store) ToggleStarted(id int64) error {
	_, err := s.db.Exec(`UPDATE tasks SET state = CASE state WHEN ? THEN ? ELSE ? END WHERE id = ? AND state != ?`,
		NotStarted, Started, NotStarted, id, Finished)
	return err
}

// ToggleFinished finishes an open task (storing the finish date) or reopens
// a finished one (removing the finish date and moving it to the bottom of the
// open tasks).
func (s *Store) ToggleFinished(id int64, now time.Time) error {
	t, err := s.GetTask(id)
	if err != nil {
		return err
	}
	if t.State != Finished {
		_, err = s.db.Exec(`UPDATE tasks SET state = ?, finished_at = ? WHERE id = ?`, Finished, now.UnixNano(), id)
		return err
	}
	pos, err := s.nextPosition()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE tasks SET state = ?, finished_at = NULL, position = ? WHERE id = ?`, NotStarted, pos, id)
	return err
}

// SwapPositions exchanges the order of two open tasks. Nothing happens if
// either of them is finished.
func (s *Store) SwapPositions(a, b int64) error {
	ta, err := s.GetTask(a)
	if err != nil {
		return err
	}
	tb, err := s.GetTask(b)
	if err != nil {
		return err
	}
	if ta.State == Finished || tb.State == Finished {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, tb.Position, a); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, ta.Position, b); err != nil {
		return err
	}
	return tx.Commit()
}

// ListProjects returns all projects ordered by name.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT id, name, active FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Active); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ActiveProjects returns the projects tasks can be assigned to.
func (s *Store) ActiveProjects() ([]Project, error) {
	all, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	var active []Project
	for _, p := range all {
		if p.Active {
			active = append(active, p)
		}
	}
	return active, nil
}

func projectErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrDuplicateName
	}
	return err
}

// CreateProject adds a new active project.
func (s *Store) CreateProject(name string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, ErrEmptyName
	}
	res, err := s.db.Exec(`INSERT INTO projects (name) VALUES (?)`, name)
	if err != nil {
		return 0, projectErr(err)
	}
	return res.LastInsertId()
}

func (s *Store) RenameProject(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}
	_, err := s.db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, name, id)
	return projectErr(err)
}

func (s *Store) SetProjectActive(id int64, active bool) error {
	_, err := s.db.Exec(`UPDATE projects SET active = ? WHERE id = ?`, active, id)
	return err
}

// DeleteProject removes a project together with all of its tasks.
func (s *Store) DeleteProject(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tasks WHERE project_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenTaskCount returns the number of unfinished tasks of a project.
func (s *Store) OpenTaskCount(projectID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE project_id = ? AND state != ?`, projectID, Finished).Scan(&n)
	return n, err
}
