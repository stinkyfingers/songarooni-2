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
	files, err := Resolve("/nonexistent", nil, 0, 0, LogoID)
	if err != nil {
		t.Fatalf("Resolve(LogoID): %v", err)
	}
	if len(files) != 1 || files[0] != "" {
		t.Fatalf("expected the lone logo marker, got %+v", files)
	}
}

func TestResolveFallsBackToLogoWhenIDNotInSongMap(t *testing.T) {
	files, err := Resolve("/nonexistent", map[int]string{1: "song-one"}, 0, 0, 999)
	if err != nil {
		t.Fatalf("Resolve(999): %v", err)
	}
	if len(files) != 1 || files[0] != "" {
		t.Fatalf("expected the logo marker for an unmapped id, got %+v", files)
	}
}

func TestResolveKnownSongReturnsItsFiles(t *testing.T) {
	root := t.TempDir()
	writeImages(t, filepath.Join(root, "song-one"), "1.png", "2.png")

	songMap := map[int]string{1: "song-one"}
	files, err := Resolve(root, songMap, 0, 0, 1)
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	sort.Strings(files)
	want := []string{filepath.Join("song-one", "1.png"), filepath.Join("song-one", "2.png")}
	if len(files) != len(want) || files[0] != want[0] || files[1] != want[1] {
		t.Fatalf("expected %v, got %v", want, files)
	}
}

func TestResolveErrorsWhenSongDirMissing(t *testing.T) {
	root := t.TempDir()
	songMap := map[int]string{1: "does-not-exist"}
	if _, err := Resolve(root, songMap, 0, 0, 1); err == nil {
		t.Fatal("expected an error when the mapped song directory doesn't exist")
	}
}

func TestResolveErrorsWhenDirectoryHasNoImages(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "song-one"), 0755); err != nil {
		t.Fatal(err)
	}

	songMap := map[int]string{1: "song-one"}
	if _, err := Resolve(root, songMap, 0, 0, 1); err == nil {
		t.Fatal("expected an error for a directory with no image files")
	}
}

func TestResolveInsertsCommercials(t *testing.T) {
	root := t.TempDir()
	writeImages(t, filepath.Join(root, "song-one"), "1.png", "2.png", "3.png", "4.png")
	writeImages(t, filepath.Join(root, "commercials"), "c1.png", "c2.png", "c3.png")

	songMap := map[int]string{1: "song-one"}
	files, err := Resolve(root, songMap, 0, 2, 1) // commercialFrequency=2 -> up to 4/2=2 commercials
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("expected 4 song files + 2 commercials, got %d: %v", len(files), files)
	}
}

func TestResolveSkipsCommercialsWhenDirectoryMissing(t *testing.T) {
	root := t.TempDir()
	writeImages(t, filepath.Join(root, "song-one"), "1.png", "2.png")

	songMap := map[int]string{1: "song-one"}
	files, err := Resolve(root, songMap, 0, 2, 1)
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected no commercials added when none exist, got %v", files)
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
