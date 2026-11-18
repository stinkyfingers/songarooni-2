package songs

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

// Reproduces the macOS AppleDouble sidecar files Finder writes alongside
// every real file (and folder) when copying to a non-HFS+/APFS filesystem
// (a FAT32/exFAT USB drive, which is how media actually gets onto the Pi)
// — "._1.jpg" alongside "1.jpg". They keep the original extension but
// aren't real image data, so they must never be treated as a slide.
func TestValidateSlideDirSkipsAppleDoubleSidecarFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "song-one", "1.jpg"), "real image bytes")
	writeFile(t, filepath.Join(dir, "song-one", "._1.jpg"), "AppleDouble metadata, not an image")

	got, err := ValidateSlideDir(dir, "song-one", 0)
	if err != nil {
		t.Fatalf("validateSlideDir: %v", err)
	}
	if len(got) != 1 || got[0].Path != filepath.Join("song-one", "1.jpg") {
		t.Fatalf("expected only the real image, got %+v", got)
	}
}

func TestValidateSlideDirErrorsWhenOnlyDotfilesPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "song-one", ".DS_Store"), "")
	writeFile(t, filepath.Join(dir, "song-one", "._1.jpg"), "")

	if _, err := ValidateSlideDir(dir, "song-one", 0); err == nil {
		t.Fatal("expected an error when every entry is a dotfile")
	}
}

// ValidateSlideDir must keep erroring on a missing directory — callers
// (in particular Slideshow.Show, for the main song deck) rely on that to
// decide whether it's safe to start cycling. A version of this once
// swallowed the error and returned an empty slice instead, which let
// Show launch cycle() with a zero-length deck and panic on deck[0].
func TestValidateSlideDirErrorsWhenDirectoryMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := ValidateSlideDir(dir, "does-not-exist", 0); err == nil {
		t.Fatal("expected an error for a missing slide directory")
	}
}

// The directory existing but holding zero valid files must also still
// error, for the same reason — this is the more realistic real-world
// state for a category like Candid, whose directory is expected to exist
// (pre-created, or created empty by the first mkdir) before anyone's
// uploaded anything.
func TestValidateSlideDirErrorsWhenDirectoryEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "empty-song"), 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateSlideDir(dir, "empty-song", 0); err == nil {
		t.Fatal("expected an error for a directory with no supported files")
	}
}
