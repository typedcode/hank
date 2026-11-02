<p align="center">
  <img src="logo.png" alt="hank" width="420">
</p>

<p align="center">
  A small, keyboard-driven terminal tool to track tasks and projects.
</p>

---

hank shows your tasks in a single table: open tasks on top in your own order, finished tasks below, newest first. Everything is done with a few keys, and all data lives in a local SQLite database.

## Features

- **Tasks** with title, optional multi-line text, due date and project
- **States** at a glance: not started, started (green), finished (gray). Overdue tasks are shown in red
- **Notes indicator**: tasks with extra text are marked with `ⓘ` in a dedicated column in the list
- **Manual ordering**: new tasks go on top, move them with `+` / `-`
- **Projects** that can be renamed, deactivated (hides their tasks) or deleted
- **Search** in title and text, plus a **project filter**, both covering all tasks
- **Infinite scrolling** through finished tasks, loaded in batches
- **Local and simple**: one SQLite file, an optional JSON config, no server

## Installation

hank needs Go 1.27 or newer.

```sh
git clone <repository-url> hank
cd hank
go install .      # installs hank into $(go env GOPATH)/bin
```

Or build a binary in the current directory with `go build .` and run `./hank`.

## Usage

Start `hank` in a terminal. The bottom line always shows the keys available in the current view.

### Task list

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | select task |
| `+` / `-` | move open task up / down |
| `s` | start / stop task |
| `f` | finish / reopen task |
| `c` | create task |
| `e` | edit task |
| `d` | delete task (with confirmation) |
| `enter` | show task details |
| `/` | search title and text |
| `F` | filter by project |
| `esc` | clear search and filter |
| `p` | manage projects |
| `q` | quit |

### Search

| Key | Action |
| --- | --- |
| type | filter the list immediately |
| `tab` | jump to the project filter |
| `enter` | leave the search field, keep the search |
| `esc` | clear the search |

### Create / edit task

| Key | Action |
| --- | --- |
| `tab` / `shift+tab` | next / previous field |
| `enter` in title | save the task |
| `enter` on project | choose a project or create a new one |
| `enter` on a button | cancel or save |
| `esc` | cancel |

Due dates use the format `yyyy-mm-dd`.

### Projects (`p`)

| Key | Action |
| --- | --- |
| `c` | create project |
| `e` | rename project |
| `a` | activate / deactivate project |
| `d` | delete project and its tasks |
| `esc` / `q` | close |

Deactivating or deleting a project asks for confirmation, and warns if the project still has open tasks.

## Configuration

The database is stored at `~/.local/share/hank/hank.db` (or `$XDG_DATA_HOME/hank/hank.db`).

An optional config file at `~/.config/hank/hank.json` (or `$XDG_CONFIG_HOME/hank/hank.json`) can change that:

```json
{
  "db_path": "~/somewhere/else/hank.db",
  "finished_page_size": 50
}
```

| Setting | Default | Description |
| --- | --- | --- |
| `db_path` | see above | location of the database file, `~` is expanded |
| `finished_page_size` | `50` | number of finished tasks loaded at once |

## Development

```sh
go test ./...     # run all tests
go vet ./...
```

The code is split into three packages:

- `internal/config` reads `hank.json` and resolves the database path
- `internal/store` holds the SQLite schema and all queries
- `internal/ui` is the terminal interface, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles) and [Lip Gloss](https://github.com/charmbracelet/lipgloss)

