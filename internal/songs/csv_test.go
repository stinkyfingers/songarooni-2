package songs

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSlideDir creates dir/<name>/001.png so a song's slide directory
// passes validation.
func writeSlideDir(t *testing.T, slideDir, name string) {
	t.Helper()
	dir := filepath.Join(slideDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "001.png"), []byte("fake png"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFileValid(t *testing.T) {
	dir := t.TempDir()
	slideDir := filepath.Join(dir, "slides")
	writeSlideDir(t, slideDir, "song-one")

	csvPath := filepath.Join(dir, "songs.csv")
	csv := "id,title,slides,tempo\n1,Song One,song-one,120\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	got, extra, err := LoadFile(csvPath, slideDir)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 || got[0].Tempo != 120 {
		t.Fatalf("unexpected songs: %+v", got)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra fields, got %v", extra)
	}
}

func TestLoadFileExtraFields(t *testing.T) {
	dir := t.TempDir()
	slideDir := filepath.Join(dir, "slides")
	writeSlideDir(t, slideDir, "song-one")

	csvPath := filepath.Join(dir, "songs.csv")
	csv := "id,title,slides,tempo,Artist,Year\n1,Song One,song-one,120,The Band,1969\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	got, extra, err := LoadFile(csvPath, slideDir)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if want := []string{"Artist", "Year"}; len(extra) != len(want) || extra[0] != want[0] || extra[1] != want[1] {
		t.Fatalf("expected extra fields %v, got %v", want, extra)
	}
	if got[0].Extra["Artist"] != "The Band" || got[0].Extra["Year"] != "1969" {
		t.Fatalf("unexpected extra data: %+v", got[0].Extra)
	}
}

func TestLoadFileRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	slideDir := filepath.Join(dir, "slides")
	writeSlideDir(t, slideDir, "song-one")
	writeSlideDir(t, slideDir, "song-two")

	csvPath := filepath.Join(dir, "songs.csv")
	csv := "id,title,slides,tempo\n1,Song One,song-one,120\n1,Song Two,song-two,100\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := LoadFile(csvPath, slideDir); err == nil {
		t.Fatal("expected an error for duplicate ids")
	}
}

// A missing slide directory is a warning, not a load failure: the song
// still loads (and falls back to the "default" deck at runtime), so a
// slides-only problem can't take the whole app — click included — down
// with it.
func TestLoadFileToleratesMissingSlideDir(t *testing.T) {
	dir := t.TempDir()
	slideDir := filepath.Join(dir, "slides")
	if err := os.MkdirAll(slideDir, 0755); err != nil {
		t.Fatal(err)
	}

	csvPath := filepath.Join(dir, "songs.csv")
	csv := "id,title,slides,tempo\n1,Song One,does-not-exist,120\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	got, _, err := LoadFile(csvPath, slideDir)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected the song to still load despite its missing slide dir: %+v", got)
	}
}

func TestLoadFileRejectsBadTempo(t *testing.T) {
	dir := t.TempDir()
	slideDir := filepath.Join(dir, "slides")
	writeSlideDir(t, slideDir, "song-one")

	csvPath := filepath.Join(dir, "songs.csv")
	csv := "id,title,slides,tempo\n1,Song One,song-one,0\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := LoadFile(csvPath, slideDir); err == nil {
		t.Fatal("expected an error for a non-positive tempo")
	}
}
