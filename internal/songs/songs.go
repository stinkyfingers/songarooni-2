package songs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Song struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Slides    string `json:"slides"`
	Tempo     int    `json:"tempo"`
	Artist    string `json:"artist,omitempty"`
	Genre     string `json:"genre,omitempty"`
	Year      string `json:"year,omitempty"`
	HasImages bool   `json:"has_images"`
}

var supportedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".mp4": true,
}

func LoadJSON(filename, slideDir string) ([]Song, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var songs []Song
	err = json.NewDecoder(file).Decode(&songs)
	if err != nil {
		return nil, err
	}

	for i, song := range songs {
		if err := validateSlideDir(slideDir, song.Slides); err != nil {
			// A broken slide directory is a slides-only problem: warn and
			// keep the song, rather than refuse to start the whole app
			// (click included) over it. slideshow.go already falls back to
			// the "default" deck at runtime when a song's directory isn't
			// there.
			log.Printf("slide validation: %s: %s", song.Title, err)
		} else {
			songs[i].HasImages = true
		}
	}
	sort.Slice(songs, func(i, j int) bool {
		return strings.ToLower(songs[i].Title) < strings.ToLower(songs[j].Title)
	})

	return songs, nil
}

// validateSlideDir confirms slideDir/slides exists and holds at least one
// supported image file.
func validateSlideDir(slideDir, slides string) error {
	dir := filepath.Join(slideDir, slides)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("slide directory not found: %s", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading slide directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if supportedImageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			return nil
		}
	}
	return fmt.Errorf("no supported image files (.png, .jpg, .jpeg, .gif) in %s", dir)
}
