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

// A row carrying a structural category (default/logo/commercials/candid)
// must stay in the catalog even when its slide directory is missing or
// empty — state.Controller.New() derives CustomIDs by scanning the
// catalog for these Category values, and if the row isn't there, that
// category's id silently stays at the zero value, which collides with
// Default's real id of 0. Candid in particular starts every show with no
// directory at all, before anyone's uploaded a first photo.
func TestLoadJSONKeepsStructuralCategoryRowsEvenWhenDirectoryMissing(t *testing.T) {
	root := t.TempDir()
	slideDir := filepath.Join(root, "media")
	songsFile := filepath.Join(root, "songs.json")
	writeFile(t, songsFile, `[{"id": 2, "title": "Candid", "slides": "candid", "category": "candid"}]`)
	// Deliberately no media/candid directory created at all.

	catalog, err := LoadJSON(songsFile, slideDir, false, 0)
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if len(catalog) != 1 || catalog[0].Category != "candid" {
		t.Fatalf("expected the candid row kept despite its missing directory, got %+v", catalog)
	}
	if catalog[0].HasImages {
		t.Fatalf("expected HasImages=false for a directory with nothing in it, got %+v", catalog[0])
	}
}

// Unlike a structural category row, a plain song with no Category and a
// broken slide directory should still be dropped from the catalog unless
// includeEmptySongs is set — the fix above must not weaken this.
func TestLoadJSONDropsBrokenPlainSongWithoutIncludeEmptySongs(t *testing.T) {
	root := t.TempDir()
	slideDir := filepath.Join(root, "media")
	songsFile := filepath.Join(root, "songs.json")
	writeFile(t, songsFile, `[{"id": 1, "title": "Real Song", "slides": "real-song", "tempo": 120}]`)

	catalog, err := LoadJSON(songsFile, slideDir, false, 0)
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if len(catalog) != 0 {
		t.Fatalf("expected a plain song with a broken slide dir to be dropped, got %+v", catalog)
	}
}
