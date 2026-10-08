package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg, defaults()) {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestLoadEmptyFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("# just a comment\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg, defaults()) {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestLoadOverridesOnlyGivenKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "addr: \":9090\"\ndisplayed_fields:\n  - Artist\n  - Year\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("expected addr override, got %q", cfg.Addr)
	}
	if cfg.SongsCSV != defaults().SongsCSV {
		t.Fatalf("expected songs_csv to keep its default, got %q", cfg.SongsCSV)
	}
	if want := []string{"Artist", "Year"}; len(cfg.DisplayedFields) != 2 || cfg.DisplayedFields[0] != want[0] || cfg.DisplayedFields[1] != want[1] {
		t.Fatalf("expected displayed_fields %v, got %v", want, cfg.DisplayedFields)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("adrr: \":9090\"\n"), 0644); err != nil { // typo: "adrr"
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown config key")
	}
}
