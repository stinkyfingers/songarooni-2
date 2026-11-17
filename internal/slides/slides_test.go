package slides

import (
	"testing"

	"songarooni/internal/songs"
)

// Resolve is now a thin lookup into songMap — the actual file-walking,
// shuffling, and duration lookup all happen once at catalog-load time in
// internal/songs — so these tests only need to confirm the lookup itself,
// not exercise a filesystem.

func TestResolveReturnsTheMappedSongsData(t *testing.T) {
	want := []songs.Data{{Path: "song-one/1.png"}, {Path: "song-one/2.png"}}
	songMap := map[int]songs.Song{
		1: {ID: 1, Title: "Song One", Data: want},
	}

	got, err := Resolve("/irrelevant", songMap, 0, 0, 1)
	if err != nil {
		t.Fatalf("Resolve(1): %v", err)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestResolveReturnsEmptyPathMarkerWhenIDNotInSongMap(t *testing.T) {
	songMap := map[int]songs.Song{1: {ID: 1, Title: "Song One"}}

	got, err := Resolve("/irrelevant", songMap, 0, 0, 999)
	if err != nil {
		t.Fatalf("Resolve(999): %v", err)
	}
	if len(got) != 1 || got[0].Path != "" {
		t.Fatalf("expected a single empty-path marker, got %+v", got)
	}
}

// insertLogo isn't called by Resolve yet (logo/commercial frequency
// insertion is reserved for a later pass — see slides.go), but the
// function itself is unchanged, so its own behavior is still worth locking
// in directly.

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
