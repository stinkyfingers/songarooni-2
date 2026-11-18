package slideshow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"songarooni/internal/songs"
	"songarooni/internal/state"
)

func recvOrTimeout(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a push on stateChan")
		return ""
	}
}

// writeSlideFile creates a real file under parentDir/subdir/name — Show now
// resolves a song's deck via a live songs.ValidateSlideDir walk (keyed off
// the song's Slides field and the Slideshow's parentDir), not a
// precomputed Data slice, so these tests need actual files on disk.
func writeSlideFile(t *testing.T, parentDir, subdir, name string) {
	t.Helper()
	dir := filepath.Join(parentDir, subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestShowPushesFirstSlideImmediately(t *testing.T) {
	parentDir := t.TempDir()
	// A single file, deliberately — ValidateSlideDir shuffles the deck,
	// so a multi-file fixture can't assert on which one comes first.
	writeSlideFile(t, parentDir, "song-one", "a.png")

	songMap := map[int]songs.Song{
		1: {ID: 1, Slides: "song-one"},
	}
	s := New(":0", parentDir, songMap, 0, 0, 0, time.Duration(1000)*time.Millisecond, "8080")

	s.Show(1)

	want := "/images/" + filepath.Join("song-one", "a.png")
	if got := recvOrTimeout(t, s.stateChan); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// A second Show call (a new song, Default, or the logo) must cancel
// whatever deck was previously cycling rather than leaving it running
// alongside the new one — see Show's own doc comment.
func TestShowCancelsPreviousCycle(t *testing.T) {
	parentDir := t.TempDir()
	writeSlideFile(t, parentDir, "song-one", "a.png")
	writeSlideFile(t, parentDir, "song-two", "x.png")

	songMap := map[int]songs.Song{
		1: {ID: 1, Slides: "song-one"},
		2: {ID: 2, Slides: "song-two"},
	}
	s := New(":0", parentDir, songMap, 0, 0, 0, time.Duration(1000)*time.Millisecond, "8080")

	s.Show(1)
	recvOrTimeout(t, s.stateChan) // consume song 1's first push

	s.Show(2)
	want := "/images/" + filepath.Join("song-two", "x.png")
	if got := recvOrTimeout(t, s.stateChan); got != want {
		t.Fatalf("expected the new deck's slide %q, got %q", want, got)
	}
}

// An id not in songMap resolves to Resolve's empty-path marker — confirm
// that still pushes something sane (no error, no panic) rather than
// silently going quiet.
func TestShowHandlesUnknownID(t *testing.T) {
	s := New(":0", "", map[int]songs.Song{}, 0, 0, 0, 0, "8080")

	s.Show(999)

	if got := recvOrTimeout(t, s.stateChan); got != "/images" {
		t.Fatalf("expected /images (the empty-path marker joined in), got %q", got)
	}
}

// A missing main-deck directory must fail safely — no push, and
// critically, no panic. ValidateSlideDir once swallowed this as an empty
// slice with a nil error, which let Show launch cycle() with a
// zero-length deck and panic on deck[0]; this guards against that
// regression recurring.
func TestShowDoesNotPanicOrCycleWhenMainDeckMissing(t *testing.T) {
	songMap := map[int]songs.Song{
		1: {ID: 1, Slides: "does-not-exist"},
	}
	s := New(":0", t.TempDir(), songMap, 0, 0, 0, time.Second, "8080")

	s.Show(1) // must not panic

	select {
	case got := <-s.stateChan:
		t.Fatalf("expected no push since the main deck failed to resolve, got %q", got)
	case <-time.After(200 * time.Millisecond):
	}
}

// Commercials/Logo/Candid are all allowed to be legitimately empty — that
// must not take the main deck down with it.
func TestShowStillDisplaysMainDeckWhenCommercialsAndLogoAreEmpty(t *testing.T) {
	parentDir := t.TempDir()
	writeSlideFile(t, parentDir, "song-one", "a.png")

	songMap := map[int]songs.Song{
		1:   {ID: 1, Slides: "song-one"},
		100: {ID: 100, Slides: "commercials-dir-never-created"},
		200: {ID: 200, Slides: "logo-dir-never-created"},
		300: {ID: 300, Slides: "candid-dir-never-created"},
	}
	s := New(":0", parentDir, songMap, 1, 1, 1, time.Second, "8080")
	s.SetCustomIDs(state.CustomIDs{CommercialID: 100, LogoID: 200, CandidID: 300})

	s.Show(1)

	want := "/images/" + filepath.Join("song-one", "a.png")
	if got := recvOrTimeout(t, s.stateChan); got != want {
		t.Fatalf("expected the main deck's slide despite empty commercials/logo/candid, got %q", got)
	}
}

// Candid interleaves into the main deck exactly like Commercials/Logo —
// confirm it actually gets pushed, on the frequency boundary.
func TestShowInterleavesCandid(t *testing.T) {
	parentDir := t.TempDir()
	writeSlideFile(t, parentDir, "song-one", "a.png")
	writeSlideFile(t, parentDir, "candid", "c1.jpg")

	songMap := map[int]songs.Song{
		1:  {ID: 1, Slides: "song-one"},
		42: {ID: 42, Slides: "candid"},
	}
	// candidEvery=1 means every main-deck slide is followed by a candid
	// one; defaultLength=0 keeps the per-slide dwell minimal so this test
	// is bounded mainly by maxRevealWait, not by also piling on dwell time.
	s := New(":0", parentDir, songMap, 0, 0, 1, 0, "8080")
	s.SetCustomIDs(state.CustomIDs{CandidID: 42})

	s.Show(1)

	wantMain := "/images/" + filepath.Join("song-one", "a.png")
	if got := recvOrTimeout(t, s.stateChan); got != wantMain {
		t.Fatalf("expected the main deck's slide first, got %q", got)
	}
	// displayData waits up to maxRevealWait for an advance POST that
	// never arrives in this unit test before the next push — give it
	// generous margin beyond that (plus crossfadePadding) rather than the
	// default 1s recvOrTimeout allows.
	select {
	case got := <-s.stateChan:
		wantCandid := "/images/" + filepath.Join("candid", "c1.jpg")
		if got != wantCandid {
			t.Fatalf("expected the interleaved candid slide %q, got %q", wantCandid, got)
		}
	case <-time.After(maxRevealWait + crossfadePadding + 3*time.Second):
		t.Fatal("timed out waiting for the interleaved candid push")
	}
}

// logoEvery/commercialEvery/candidEvery of 0 means "disabled" (see
// config.go's doc comments) — i % 0 panics, so cycle() must check this
// before the modulo, not rely on len(...) > 0 alone to skip it.
func TestShowDoesNotPanicWhenFrequenciesAreZero(t *testing.T) {
	parentDir := t.TempDir()
	writeSlideFile(t, parentDir, "song-one", "a.png")

	songMap := map[int]songs.Song{1: {ID: 1, Slides: "song-one"}}
	s := New(":0", parentDir, songMap, 0, 0, 0, time.Millisecond, "8080")

	s.Show(1) // must not panic on the first (or any) iteration

	want := "/images/" + filepath.Join("song-one", "a.png")
	if got := recvOrTimeout(t, s.stateChan); got != want {
		t.Fatalf("expected the main deck's slide, got %q", got)
	}
}
