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
	Addr                 string  `yaml:"addr" json:"addr"`                                     // HTTP listen address, e.g. ":8080"
	SongsFile            string  `yaml:"songs_file" json:"songs_file"`                         // path to songs.json
	SlideDir             string  `yaml:"slide_dir" json:"slide_dir"`                           // base directory containing per-song slide subdirectories
	DefaultSlideshow     string  `yaml:"default_slideshow" json:"default_slideshow"`           // directory of non-song-specific slides: the Default button, and the fallback for an unknown/missing song deck
	SlideIntervalSeconds int     `yaml:"slide_interval_seconds" json:"slide_interval_seconds"` // seconds per slide before crossfading to the next
	LogoFrequency        int     `yaml:"logo_frequency" json:"logo_frequency"`                 // insert the logo after every N slides (<=0 disables it)
	CommercialFrequency  int     `yaml:"commercial_frequency" json:"commercial_frequency"`     // insert a commercial after every N slides (<=0 disables it)
	AudioDevice          string  `yaml:"audio_device" json:"audio_device"`                     // ALSA device name for the click, e.g. "default" or "hw:1,0"
	ClickVolume          float64 `yaml:"click_volume" json:"click_volume"`                     // click loudness, 0 (silent) to 1 (full scale)
	ClickFrequency       float64 `yaml:"click_frequency" json:"click_frequency"`               // click tone frequency in Hz, e.g. 1800
	LogFile              string  `yaml:"log_file" json:"log_file"`                             // path to the truncated-on-startup log file
	NoKiosk              bool    `yaml:"no_kiosk" json:"no_kiosk"`                             // if true, launch a regular, non-kiosk browser instead of the slideshow kiosk
	NoClick              bool    `yaml:"no_click" json:"no_click"`
	IncludeEmptySongs    bool    `yaml:"include_empty_songs" json:"include_empty_songs"` // if true, include songs with no images

	// PedalDeviceName is matched case-insensitively against each /dev/input
	// device's name (via EVIOCGNAME) to find the M-Vave Chocolate foot
	// pedal — see internal/pedal. "Chocolate" is a best guess, not a
	// confirmed value; if the pedal isn't found, check its real name with
	// `cat /proc/bus/input/devices` on the Pi and set this to match.
	PedalDeviceName string `yaml:"pedal_device_name" json:"pedal_device_name"`

	// BrowserSlideshowAddr is the slideshow kiosk page's own HTTP listen
	// address — deliberately separate from Addr (the control panel's), not
	// merged into the same mux.
	BrowserSlideshowAddr string `yaml:"browser_slideshow_addr" json:"browser_slideshow_addr"`

	// BrowserCommand is the kiosk browser's full command + args, with
	// "{{url}}" replaced at launch time by the slideshow page's URL.
	// Different browsers want the URL in different places (Chromium's
	// --app= fuses it into one flag; others take it as a bare trailing
	// arg), which a placeholder handles generically. Leave empty (the
	// default) to auto-detect a Chromium-family browser on PATH instead —
	// see internal/slideshow.
	BrowserCommand []string `yaml:"browser_command" json:"browser_command"`
}

func defaults() Config {
	return Config{
		Addr:                 ":8080",
		SongsFile:            "songs.json",
		SlideDir:             "media/slides",
		DefaultSlideshow:     "media/default",
		SlideIntervalSeconds: 20,
		LogoFrequency:        5,
		CommercialFrequency:  5,
		AudioDevice:          "default",
		ClickVolume:          1.0,
		ClickFrequency:       1800,
		LogFile:              "songarooni.log",
		PedalDeviceName:      "FootCtrlPlus Keyboard",
		BrowserSlideshowAddr: ":8888",
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
