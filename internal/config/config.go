// Package config loads config.yaml.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Addr                 string   `yaml:"addr"`                   // HTTP listen address, e.g. ":8080"
	SongsCSV             string   `yaml:"songs_csv"`              // path to songs.csv
	SlideDir             string   `yaml:"slide_dir"`              // base directory containing per-song slide subdirectories
	DefaultSlideshow     string   `yaml:"default_slideshow"`      // directory of non-song-specific slides: the Default button, and the fallback for an unknown/missing song deck
	SlideIntervalSeconds int      `yaml:"slide_interval_seconds"` // feh: seconds per slide / reload interval
	LogoFrequency        int      `yaml:"logo_frequency"`         // feh: insert the logo after every N slides
	AudioDevice          string   `yaml:"audio_device"`           // ALSA device name for the click, e.g. "default" or "hw:1,0"
	LogFile              string   `yaml:"log_file"`               // path to the truncated-on-startup log file
	DisplayedFields      []string `yaml:"displayed_fields"`       // which extra songs.csv columns to show in the phone's song list (empty: none)
}

func defaults() Config {
	return Config{
		Addr:                 ":8080",
		SongsCSV:             "songs.csv",
		SlideDir:             "media/slides",
		DefaultSlideshow:     "media/default",
		SlideIntervalSeconds: 20,
		LogoFrequency:        5,
		AudioDevice:          "default",
		LogFile:              "songarooni.log",
	}
}

// Load reads path if it exists, overriding defaults field by field; a
// missing file just means "use the defaults", which keeps `go run` working
// with no config.yaml at all. Unknown keys are rejected rather than
// silently ignored, so a typo in config.yaml doesn't fail quietly.
func Load(path string) (Config, error) {
	cfg := defaults()

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		// io.EOF means the file exists but has no document in it (empty,
		// or comments only) — that's just "use the defaults", not an error.
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}
