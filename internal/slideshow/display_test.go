package slideshow

import (
	"testing"
	"time"

	"songarooni/internal/songs"
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

func TestShowPushesFirstSlideImmediately(t *testing.T) {
	songMap := map[int]songs.Song{
		1: {ID: 1, Data: []songs.Data{{Path: "a.png"}, {Path: "b.png"}}},
	}
	s := New(":0", "", songMap, 0, 0, "8080")

	s.Show(1)

	if got := recvOrTimeout(t, s.stateChan); got != "/images/a.png" {
		t.Fatalf("expected /images/a.png, got %q", got)
	}
}

// A second Show call (a new song, Default, or the logo) must cancel
// whatever deck was previously cycling rather than leaving it running
// alongside the new one — see Show's own doc comment.
func TestShowCancelsPreviousCycle(t *testing.T) {
	songMap := map[int]songs.Song{
		1: {ID: 1, Data: []songs.Data{{Path: "a.png"}}},
		2: {ID: 2, Data: []songs.Data{{Path: "x.png"}}},
	}
	s := New(":0", "", songMap, 0, 0, "8080")

	s.Show(1)
	recvOrTimeout(t, s.stateChan) // consume song 1's first push

	s.Show(2)
	if got := recvOrTimeout(t, s.stateChan); got != "/images/x.png" {
		t.Fatalf("expected the new deck's slide /images/x.png, got %q", got)
	}
}

// An id not in songMap resolves to slides.Resolve's empty-path marker —
// confirm that still pushes something sane (no error, no panic) rather
// than silently going quiet.
func TestShowHandlesUnknownID(t *testing.T) {
	s := New(":0", "", map[int]songs.Song{}, 0, 0, "8080")

	s.Show(999)

	if got := recvOrTimeout(t, s.stateChan); got != "/images" {
		t.Fatalf("expected /images (the empty-path marker joined in), got %q", got)
	}
}
