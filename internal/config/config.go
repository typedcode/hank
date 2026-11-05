// Package config loads the optional hank configuration file and resolves
// the location of the database.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultFinishedPageSize is the number of finished tasks loaded at once.
const DefaultFinishedPageSize = 50

// DefaultDueSoonDays is the number of days before its due date from which on
// a task is shown as due soon.
const DefaultDueSoonDays = 3

// Config holds the settings from hank.json. Every field is optional.
type Config struct {
	DBPath           string `json:"db_path,omitempty"`
	FinishedPageSize int    `json:"finished_page_size,omitempty"`
	DueSoonDays      *int   `json:"due_soon_days,omitempty"` // nil means default, 0 is a valid value
}

// PageSize returns the configured number of finished tasks to load at once,
// or the default.
func (c Config) PageSize() int {
	if c.FinishedPageSize <= 0 {
		return DefaultFinishedPageSize
	}
	return c.FinishedPageSize
}

// DueSoon returns the configured number of days before its due date from
// which on a task is shown as due soon, or the default.
func (c Config) DueSoon() int {
	if c.DueSoonDays == nil || *c.DueSoonDays < 0 {
		return DefaultDueSoonDays
	}
	return *c.DueSoonDays
}

// Path returns the location of the configuration file:
// $XDG_CONFIG_HOME/hank/hank.json, falling back to ~/.config/hank/hank.json.
func Path() (string, error) {
	return xdgPath("XDG_CONFIG_HOME", ".config", "hank.json")
}

// DefaultDBPath returns $XDG_DATA_HOME/hank/hank.db, falling back to
// ~/.local/share/hank/hank.db.
func DefaultDBPath() (string, error) {
	return xdgPath("XDG_DATA_HOME", filepath.Join(".local", "share"), "hank.db")
}

func xdgPath(env, fallback, file string) (string, error) {
	base := os.Getenv(env)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(base, "hank", file), nil
}

// Load reads the configuration file from Path. A missing file is not an
// error and yields the zero Config.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return LoadFile(path)
}

// LoadFile reads the configuration from path. A missing file yields the zero
// Config; a file that cannot be parsed is an error.
func LoadFile(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to the configuration file at Path.
func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveFile(path, cfg)
}

// SaveFile writes cfg to path, creating its directory if necessary. Unset
// settings are left out, so they keep following the defaults.
func SaveFile(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ResolveDBPath returns the configured database path with a leading ~
// expanded, or the default path if none is configured.
func (c Config) ResolveDBPath() (string, error) {
	if c.DBPath == "" {
		return DefaultDBPath()
	}
	return expandHome(c.DBPath)
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
