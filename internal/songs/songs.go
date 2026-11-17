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
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Slides string `json:"slides"`
	Tempo  int    `json:"tempo"`
	Artist string `json:"artist,omitempty"`
	Genre  string `json:"genre,omitempty"`
	Year   string `json:"year,omitempty"`

	HasData bool   `json:"has_data"` // true if the song's slides dir exists and has at least one supported image file
	Data    []Data `json:"data,omitempty"`
}

type Data struct {
	SongID   int     `json:"song_id"`
	Type     string  `json:"type"` // "image" or "video"
	Filename string  `json:"filename"`
	Duration float64 `json:"duration,omitempty"` // seconds, for videos
}

var supportedImageExts = map[string]string{
	".png": "image", ".jpg": "image", ".jpeg": "image", ".gif": "image", ".mp4": "video",
}

func LoadJSON(filename, slideDir string, includeEmptySongs bool) ([]Song, error) {
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

	var filteredSongs []Song

	for i, song := range songs {
		if data, err := validateSlideDir(slideDir, song.Slides); err != nil {
			log.Printf("song data validation: %s: %s", song.Title, err)

			if includeEmptySongs {
				songs[i].HasData = false
				filteredSongs = append(filteredSongs, songs[i])
			}
		} else {
			songs[i].HasData = true
			songs[i].Data = data
			filteredSongs = append(filteredSongs, songs[i])
		}
	}
	sort.Slice(filteredSongs, func(i, j int) bool {
		return strings.ToLower(filteredSongs[i].Title) < strings.ToLower(filteredSongs[j].Title)
	})

	return filteredSongs, nil
}

// validateSlideDir confirms slideDir/slides exists and holds at least one
// supported data file.
func validateSlideDir(slideDir, slides string) ([]Data, error) {
	dir := filepath.Join(slideDir, slides)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("slide directory not found: %s", dir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading slide directory %s: %w", dir, err)
	}
	var data []Data
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if typ, ok := supportedImageExts[strings.ToLower(filepath.Ext(e.Name()))]; ok {
			data = append(data, Data{
				Type:     typ,
				Filename: e.Name(),
			})
			if typ == "video" {
				dur, err := MP4Duration(filepath.Join(dir, e.Name()))
				if err != nil {
					log.Printf("reading %s duration: %v; skipping duration", e.Name(), err)
				} else {
					data[len(data)-1].Duration = dur.Seconds()
				}
			}
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no supported data files in %s", dir)
	}
	return data, nil
}
