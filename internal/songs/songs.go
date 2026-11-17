package songs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Song struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Category  string `json:"category,omitempty"`
	Slides    string `json:"slides"`
	Tempo     int    `json:"tempo"`
	Artist    string `json:"artist,omitempty"`
	Genre     string `json:"genre,omitempty"`
	Year      string `json:"year,omitempty"`
	HasImages bool   `json:"has_images"`
	Data      []Data `json:"data,omitempty"`
}

type Data struct {
	Path   string        `json:"path"`
	Length time.Duration `json:"length"`
}

var supportedImageExts = map[string]string{
	".png": "image", ".jpg": "image", ".jpeg": "image", ".gif": "image", ".mp4": "video",
}

func LoadJSON(filename, slideDir string, includeEmptySongs bool, defaultLength time.Duration) ([]Song, error) {
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
		if dataList, err := validateSlideDir(slideDir, song.Slides, defaultLength); err != nil {
			// A broken slide directory is a slides-only problem: warn and
			// keep the song, rather than refuse to start the whole app
			// (click included) over it. slideshow.go already falls back to
			// the "default" deck at runtime when a song's directory isn't
			// there.
			log.Printf("slide validation: %s: %s", song.Title, err)

			if includeEmptySongs {
				songs[i].HasImages = false
				filteredSongs = append(filteredSongs, songs[i])
			}
		} else {
			songs[i].HasImages = true
			songs[i].Data = dataList
			filteredSongs = append(filteredSongs, songs[i])
		}
	}
	sort.Slice(filteredSongs, func(i, j int) bool {
		return strings.ToLower(filteredSongs[i].Title) < strings.ToLower(filteredSongs[j].Title)
	})

	return filteredSongs, nil
}

// validateSlideDir confirms slideDir/slides exists and holds at least one
// supported image file.
func validateSlideDir(slideDir, slides string, defaultLength time.Duration) ([]Data, error) {
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
	var dataList []Data
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip dotfiles — in particular macOS's AppleDouble sidecar files
		// (e.g. "._1.jpg" alongside "1.jpg"), written automatically by
		// Finder for every copied file/folder when the destination isn't
		// HFS+/APFS (a FAT32/exFAT USB drive, which is how media actually
		// gets onto the Pi). They keep the original extension but aren't
		// real image/video data, so letting one through here means the
		// browser gets a URL that silently fails to load.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if typ, ok := supportedImageExts[strings.ToLower(filepath.Ext(e.Name()))]; ok {
			length := defaultLength
			if typ == "video" {
				length, err = MP4Duration(filepath.Join(dir, e.Name()))
				if err != nil {
					log.Println("slide validation: error reading video duration for", e.Name(), ":", err)
				}
			}
			dataList = append(dataList, Data{
				Path:   filepath.Join(slides, e.Name()), // relative to slideDir, not absolute
				Length: length,
			})
		}
	}
	if len(dataList) == 0 {
		return dataList, fmt.Errorf("no supported image files (.png, .jpg, .jpeg, .gif) in %s", dir)
	}
	return dataList, nil
}
