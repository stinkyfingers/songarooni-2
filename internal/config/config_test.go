package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("adrr: \":9090\"\n"), 0644); err != nil { // typo: "adrr"
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown config key")
	}
}

func TestLoadReadsCameraCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "camera_username: alice\ncamera_password: secret123\ncamera_ip: 192.168.1.42\ncamera_port: 8080\n"
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CameraUsername != "alice" || cfg.CameraPassword != "secret123" {
		t.Fatalf("expected credentials to load from YAML, got %+v", cfg)
	}
	if cfg.CameraIP != "192.168.1.42" || cfg.CameraPort != 8080 {
		t.Fatalf("expected camera_ip/camera_port to load from YAML, got %+v", cfg)
	}
}

// Confirms the json:"-" tags on CameraUsername/CameraPassword actually do
// what they're there for: GET /api/config (control_panel.go) serializes
// the whole Config struct verbatim, so a camera password must never
// survive a JSON round-trip or it leaks to every phone on the LAN.
func TestCameraCredentialsNeverMarshalToJSON(t *testing.T) {
	cfg := defaults()
	cfg.CameraUsername = "alice"
	cfg.CameraPassword = "super-secret"

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "alice") || strings.Contains(string(data), "super-secret") {
		t.Fatalf("camera credentials leaked into JSON output: %s", data)
	}
}
