package slides

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeImages(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveLogoID(t *testing.T) {
	deck, err := Resolve("/nonexistent", "/nonexistent", nil, 0, LogoID)
	if err != nil {
		t.Fatalf("Resolve(LogoID): %v", err)
	}
	if len(deck.Files) != 1 || deck.Files[0] != "" {
		t.Fatalf("expected the lone logo marker, got %+v", deck)
	}
}

func TestResolveDefaultID(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default")
	writeImages(t, defaultDir, "a.png", "b.jpg")

	deck, err := Resolve(filepath.Join(root, "songs"), defaultDir, nil, 0, DefaultID)
	if err != nil {
		t.Fatalf("Resolve(DefaultID): %v", err)
	}
	if deck.Base != DefaultRoot {
		t.Fatalf("expected DefaultRoot, got %v", deck.Base)
	}
	if len(deck.Files) != 2 {
		t.Fatalf("expected 2 files, got %+v", deck.Files)
	}
}

func TestResolveKnownSongUsesItsOwnDirectory(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "songs")
	defaultDir := filepath.Join(root, "default")
	writeImages(t, filepath.Join(parentDir, "song-one"), "1.png", "2.png")
	writeImages(t, defaultDir, "d.png")

	songMap := map[int]string{1: "song-one"}
	deck, err := Resolve(parentDir, defaultDir, songMap, 0, 1)
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	if deck.Base != SongRoot || deck.Subdir != "song-one" {
		t.Fatalf("expected SongRoot/song-one, got %+v", deck)
	}
	got := append([]string(nil), deck.Files...)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "1.png" || got[1] != "2.png" {
		t.Fatalf("expected [1.png 2.png], got %v", got)
	}
}

// A song's own deck keeps its natural (alphabetical) order — only the
// default deck is randomized. With Go's shuffle seeded by the runtime, the
// only reliable way to assert "not shuffled" is exact order equality,
// repeated to rule out coincidental matches.
func TestResolveSongDeckIsNotShuffled(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "songs")
	defaultDir := filepath.Join(root, "default")
	writeImages(t, filepath.Join(parentDir, "song-one"), "1.png", "2.png", "3.png", "4.png", "5.png")
	writeImages(t, defaultDir, "d.png")

	songMap := map[int]string{1: "song-one"}
	for i := 0; i < 10; i++ {
		deck, err := Resolve(parentDir, defaultDir, songMap, 0, 1)
		if err != nil {
			t.Fatalf("Resolve(1): %v", err)
		}
		want := []string{"1.png", "2.png", "3.png", "4.png", "5.png"}
		if len(deck.Files) != len(want) {
			t.Fatalf("expected %v, got %v", want, deck.Files)
		}
		for i, f := range want {
			if deck.Files[i] != f {
				t.Fatalf("expected song deck in natural order %v, got %v", want, deck.Files)
			}
		}
	}
}

func TestResolveFallsBackToDefaultWhenSongDirMissing(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "songs")
	defaultDir := filepath.Join(root, "default")
	writeImages(t, defaultDir, "d.png")
	// No directory created for song id 1's mapped subdirectory.
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		t.Fatal(err)
	}

	songMap := map[int]string{1: "does-not-exist"}
	deck, err := Resolve(parentDir, defaultDir, songMap, 0, 1)
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	if deck.Base != DefaultRoot {
		t.Fatalf("expected fallback to DefaultRoot, got %+v", deck)
	}
}

func TestResolveFallsBackToDefaultWhenIDNotInSongMap(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "songs")
	defaultDir := filepath.Join(root, "default")
	writeImages(t, defaultDir, "d.png")
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		t.Fatal(err)
	}

	deck, err := Resolve(parentDir, defaultDir, map[int]string{}, 0, 999)
	if err != nil {
		t.Fatalf("Resolve(999): %v", err)
	}
	if deck.Base != DefaultRoot {
		t.Fatalf("expected fallback to DefaultRoot for an unmapped id, got %+v", deck)
	}
}

func TestResolveErrorsWhenDirectoryHasNoImages(t *testing.T) {
	root := t.TempDir()
	defaultDir := filepath.Join(root, "default")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := Resolve(filepath.Join(root, "songs"), defaultDir, nil, 0, DefaultID); err == nil {
		t.Fatal("expected an error for a directory with no image files")
	}
}

func TestInsertLogoFrequency(t *testing.T) {
	files := []string{"1.png", "2.png", "3.png", "4.png", "5.png"}

	got := insertLogo(files, 2)
	want := []string{"1.png", "2.png", "", "3.png", "4.png", "", "5.png"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, f := range want {
		if got[i] != f {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestInsertLogoDisabledWhenFrequencyIsZeroOrLess(t *testing.T) {
	files := []string{"1.png", "2.png"}
	for _, freq := range []int{0, -1} {
		got := insertLogo(files, freq)
		if len(got) != len(files) || got[0] != files[0] || got[1] != files[1] {
			t.Fatalf("logoFrequency=%d: expected files unchanged, got %v", freq, got)
		}
	}
}
