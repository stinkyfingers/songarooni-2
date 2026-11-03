// Package slides resolves a song id to its ordered slide deck for
// internal/slideshow's kiosk page — the DefaultID/LogoID/missing-directory/
// shuffle/logo-insertion rules live here, independent of the HTTP serving
// that consumes them.
package slides

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

// DefaultID is the sentinel song id that selects the "default" subdirectory
// instead of a specific song's slides. Real song ids from songs.csv start at
// 1, so 0 is never a valid song id.
const DefaultID = 0

// LogoID is the sentinel id that shows just the logo, full-screen, instead
// of any song's slide deck. Negative, so it can never collide with a real
// song id (which must be positive) or DefaultID.
const LogoID = -1

// Root identifies which configured directory a Deck's slides came from, so
// a caller can map it to whatever representation it needs (an absolute
// filesystem path for feh's img-list.txt, a bounded URL prefix for the
// browser endpoint) without this package knowing about either.
type Root int

const (
	SongRoot    Root = iota // parentDir/Subdir
	DefaultRoot             // defaultDir
)

// Deck is one resolved song's ordered slides. Files is a list of filenames
// relative to whichever root Base identifies (Subdir additionally joined in
// for SongRoot); an empty string entry is a logo slide, not a real filename.
// A LogoID resolution is the single-entry deck {Files: [""]} — Base/Subdir
// are meaningless for it.
type Deck struct {
	Base   Root
	Subdir string // only meaningful when Base == SongRoot
	Files  []string
}

var supportedImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".mp4": true,
}

// Resolve picks id's slide deck: LogoID is just the logo on its own;
// DefaultID, an id with no entry in songMap, or a mapped id whose directory
// doesn't exist under parentDir all fall back to defaultDir; otherwise
// parentDir/songMap[id] is used. Only the default deck is shuffled — a
// song's own slides keep their natural (alphabetical) order, matching
// internal/slideshow's existing behavior, which only ever randomized the
// fallback deck. If logoFrequency > 0, a logo slide is inserted after every
// logoFrequency images.
func Resolve(parentDir, defaultDir string, songMap map[int]string, logoFrequency, id int) (Deck, error) {
	if id == LogoID {
		return Deck{Files: []string{""}}, nil
	}

	root := DefaultRoot
	subdir := ""
	dir := defaultDir
	if sub, ok := songMap[id]; id != DefaultID && ok {
		candidate := filepath.Join(parentDir, sub)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			root = SongRoot
			subdir = sub
			dir = candidate
		}
	}

	files, err := imageFiles(dir)
	if err != nil {
		return Deck{}, err
	}

	if root == DefaultRoot {
		rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
	}

	return Deck{Base: root, Subdir: subdir, Files: insertLogo(files, logoFrequency)}, nil
}

// imageFiles lists dir's supported image files (recursively, like the
// previous feh-only implementation), as paths relative to dir, in the
// filesystem's natural walk order (alphabetical per directory level).
// Hidden files (dotfiles) are skipped.
func imageFiles(dir string) ([]string, error) {
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
		files = append(files, rel)
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
