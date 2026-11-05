package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMissing(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoadFileInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hank.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadFileAndResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(t.TempDir(), "hank.json")
	os.WriteFile(path, []byte(`{"db_path": "~/data/tasks.db", "finished_page_size": 20}`), 0o644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.ResolveDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "data", "tasks.db"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if cfg.PageSize() != 20 {
		t.Fatalf("page size: %d", cfg.PageSize())
	}
	if (Config{}).PageSize() != DefaultFinishedPageSize {
		t.Fatal("default page size not used")
	}
}

func TestSaveFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hank.json")
	want := Config{DBPath: "~/data/tasks.db", FinishedPageSize: 20}
	if err := SaveFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	if err := SaveFile(path, Config{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{}\n" {
		t.Fatalf("unset settings written: %s", data)
	}
}

func TestDueSoon(t *testing.T) {
	if (Config{}).DueSoon() != DefaultDueSoonDays {
		t.Fatal("default not used")
	}
	path := filepath.Join(t.TempDir(), "hank.json")
	zero := 0
	if err := SaveFile(path, Config{DueSoonDays: &zero}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DueSoon() != 0 {
		t.Fatalf("explicit 0 lost: %d", cfg.DueSoon())
	}
	negative := -1
	if (Config{DueSoonDays: &negative}).DueSoon() != DefaultDueSoonDays {
		t.Fatal("negative value used")
	}
}

func TestDefaultPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")

	db, _ := Config{}.ResolveDBPath()
	if want := filepath.Join(home, ".local", "share", "hank", "hank.db"); db != want {
		t.Errorf("db path: got %q, want %q", db, want)
	}
	cfg, _ := Path()
	if want := "/xdg/config/hank/hank.json"; cfg != want {
		t.Errorf("config path: got %q, want %q", cfg, want)
	}
}
