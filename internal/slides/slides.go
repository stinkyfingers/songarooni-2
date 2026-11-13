// Package slides resolves a song id to its ordered slide deck for
// internal/slideshow's kiosk page — the DefaultID/LogoID/missing-directory/
// shuffle/logo-insertion rules live here, independent of the HTTP serving
// that consumes them.
package slides

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the sentinel song id that selects the "default" subdirectory
// instead of a specific song's slides. Real song ids from songs file start at
// 1, so 0 is never a valid song id.
const DefaultID = 0

// LogoID is the sentinel id that shows just the logo, full-screen, instead
// of any song's slide deck. Negative, so it can never collide with a real
// song id (which must be positive) or DefaultID.
const LogoID = -1

var supportedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".mp4": true,
}

// Resolve picks id's slide deck.
func Resolve(parentDir string, songMap map[int]string, logoFrequency, commercialFrequency, id int) ([]string, error) {
	if id == LogoID {
		return []string{""}, nil
	}

	songDir, ok := songMap[id]
	if !ok {
		log.Println("slides: song id", id, "not found in songMap; falling back to default")
		return []string{""}, nil
	}
	files, err := imageFiles(parentDir, songDir)
	if err != nil {
		return nil, err
	}

	files = insertCommercials(parentDir, files, commercialFrequency)
	files = insertLogo(files, logoFrequency)

	rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	fmt.Println("files", files) // TODO cleanup

	log.Println("playing slides in", songDir)
	return files, nil
}

// imageFiles lists dir's supported image files (recursively, like the
// previous feh-only implementation), as paths relative to dir, in the
// filesystem's natural walk order (alphabetical per directory level).
// Hidden files (dotfiles) are skipped.
func imageFiles(parentDir, songDir string) ([]string, error) {
	dir := filepath.Join(parentDir, songDir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("image directory not found: %s", dir)
	}

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if !supportedImageExts[strings.ToLower(filepath.Ext(info.Name()))] {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.Join(songDir, rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no image files found in %s", dir)
	}
	return files, nil
}

// insertLogo splices a "" (logo marker) after every logoFrequency images.
// logoFrequency <= 0 disables insertion, returning files unchanged.
func insertLogo(files []string, logoFrequency int) []string {
	if logoFrequency <= 0 {
		return files
	}
	out := make([]string, 0, len(files)+len(files)/logoFrequency)
	for i, f := range files {
		out = append(out, f)
		if (i+1)%logoFrequency == 0 {
			out = append(out, "")
		}
	}
	return out
}

func insertCommercials(parentDir string, files []string, commercialFrequency int) []string {
	commercials, err := imageFiles(parentDir, "commercials")
	if err != nil {
		log.Printf("slides: no commercials directory found under %s: %v", parentDir, err)
	} else {
		// select arbitrary commercials to insert
		max := len(files) / commercialFrequency
		if max > len(commercials) {
			max = len(commercials)
		}
		if max > 0 {
			rand.Shuffle(len(commercials), func(i, j int) { commercials[i], commercials[j] = commercials[j], commercials[i] })
			commercials = commercials[:max]
		} else {
			commercials = nil
		}
		files = append(files, commercials...)
	}
	return files
}
