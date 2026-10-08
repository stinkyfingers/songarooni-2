// Package songs loads and validates the song catalog from songs.csv.
package songs

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Song is one row of songs.csv.
type Song struct {
	ID     int
	Title  string
	Slides string // subdirectory name under the configured slide_dir
	Tempo  int    // beats per minute

	// Extra holds any CSV columns beyond id/title/slides/tempo, keyed by
	// their header name (e.g. "Artist", "Year") — free-form metadata for
	// display, search, and sort, not used by the app's own logic.
	Extra map[string]string
}

var supportedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
}

var requiredHeader = []string{"id", "title", "slides", "tempo"}

// LoadFile parses path as a songs.csv file and validates every row: unique
// ids, nonempty titles, a positive tempo, and a slide_dir/<slides>
// subdirectory that exists and contains at least one supported image file.
// It returns every problem found rather than stopping at the first one, so a
// broken catalog can be fixed in one pass instead of one error at a time.
//
// The header must start with id,title,slides,tempo; any columns after that
// are accepted as free-form extra fields (see Song.Extra), in header order.
func LoadFile(path, slideDir string) (songList []Song, extraFields []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("%s is empty", path)
	}

	extraFields = []string{} // never nil: the API serializes this as JSON, where null and [] aren't interchangeable

	header := rows[0]
	if len(header) < len(requiredHeader) {
		return nil, nil, fmt.Errorf("%s: expected header to start with %v, got %v", path, requiredHeader, header)
	}
	for i, col := range requiredHeader {
		if strings.TrimSpace(header[i]) != col {
			return nil, nil, fmt.Errorf("%s: expected header to start with %v, got %v", path, requiredHeader, header)
		}
	}
	for _, col := range header[len(requiredHeader):] {
		name := strings.TrimSpace(col)
		if name == "" {
			return nil, nil, fmt.Errorf("%s: empty extra column name in header %v", path, header)
		}
		extraFields = append(extraFields, name)
	}

	var (
		errs    []string
		seenIDs = map[int]bool{}
	)

	for i, row := range rows[1:] {
		line := i + 2 // 1-indexed, plus the header row
		if len(row) != len(header) {
			errs = append(errs, fmt.Sprintf("line %d: expected %d columns, got %d", line, len(header), len(row)))
			continue
		}

		id, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil || id <= 0 {
			errs = append(errs, fmt.Sprintf("line %d: invalid id %q (must be a positive integer)", line, row[0]))
			continue
		}
		if seenIDs[id] {
			errs = append(errs, fmt.Sprintf("line %d: duplicate id %d", line, id))
			continue
		}

		title := strings.TrimSpace(row[1])
		if title == "" {
			errs = append(errs, fmt.Sprintf("line %d: empty title", line))
			continue
		}

		slides := strings.TrimSpace(row[2])
		if slides == "" {
			errs = append(errs, fmt.Sprintf("line %d (%s): empty slides directory", line, title))
			continue
		}
		if err := validateSlideDir(slideDir, slides); err != nil {
			// A broken slide directory is a slides-only problem: warn and
			// keep the song, rather than refuse to start the whole app
			// (click included) over it. slideshow.go already falls back to
			// the "default" deck at runtime when a song's directory isn't
			// there.
			log.Printf("songs: line %d (%s): %s — will fall back to the default slide deck at runtime", line, title, err)
		}

		tempo, err := strconv.Atoi(strings.TrimSpace(row[3]))
		if err != nil || tempo <= 0 {
			errs = append(errs, fmt.Sprintf("line %d (%s): invalid tempo %q", line, title, row[3]))
			continue
		}

		var extra map[string]string
		if len(extraFields) > 0 {
			extra = make(map[string]string, len(extraFields))
			for i, name := range extraFields {
				extra[name] = strings.TrimSpace(row[len(requiredHeader)+i])
			}
		}

		seenIDs[id] = true
		songList = append(songList, Song{ID: id, Title: title, Slides: slides, Tempo: tempo, Extra: extra})
	}

	if len(errs) > 0 {
		return nil, nil, fmt.Errorf("%s has %d problem(s):\n%s", path, len(errs), strings.Join(errs, "\n"))
	}
	return songList, extraFields, nil
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
