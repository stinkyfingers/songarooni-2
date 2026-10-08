package state

import (
	"errors"
	"testing"

	"songarooni/internal/slideshow"
	"songarooni/internal/songs"
)

type fakeClicker struct {
	running   bool
	starts    int
	stops     int
	lastTempo int
}

func (f *fakeClicker) Start(tempo int) error {
	if f.running {
		return errors.New("fake: already running")
	}
	f.running = true
	f.starts++
	f.lastTempo = tempo
	return nil
}

func (f *fakeClicker) Stop() error {
	f.running = false
	f.stops++
	return nil
}

type fakeDisplay struct {
	shown []int
}

func (f *fakeDisplay) Show(id int) {
	f.shown = append(f.shown, id)
}

func testCatalog() []songs.Song {
	return []songs.Song{
		{ID: 1, Title: "Honky Tonk Women", Slides: "honky-tonk-women", Tempo: 118},
		{ID: 2, Title: "The Weight", Slides: "the-weight", Tempo: 112},
	}
}

func TestSelectRequiresKnownID(t *testing.T) {
	c := New(testCatalog(), &fakeClicker{}, &fakeDisplay{})
	if err := c.SelectSong(999); err == nil {
		t.Fatal("expected an error selecting an unknown id")
	}
}

func TestStartRequiresSelection(t *testing.T) {
	c := New(testCatalog(), &fakeClicker{}, &fakeDisplay{})
	if err := c.Start(); err == nil {
		t.Fatal("expected an error starting with no song selected")
	}
}

func TestSelectStartStop(t *testing.T) {
	clicker := &fakeClicker{}
	display := &fakeDisplay{}
	c := New(testCatalog(), clicker, display)

	if err := c.SelectSong(1); err != nil {
		t.Fatalf("SelectSong: %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := c.GetState()
	if !got.Running || got.PlayingSongID != 1 || got.SelectedSongID != 1 {
		t.Fatalf("unexpected state after Start: %+v", got)
	}
	if !clicker.running || clicker.lastTempo != 118 {
		t.Fatalf("expected the clicker running at 118 BPM, got %+v", clicker)
	}
	if len(display.shown) != 1 || display.shown[0] != 1 {
		t.Fatalf("expected display.Show(1), got %v", display.shown)
	}

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if clicker.running {
		t.Fatal("expected the clicker to be stopped")
	}
	got = c.GetState()
	if got.Running {
		t.Fatal("expected Running=false after Stop")
	}
	if got.SelectedSongID != 1 {
		t.Fatal("expected the selection to survive Stop")
	}
}

func TestDoubleStartIsRejected(t *testing.T) {
	c := New(testCatalog(), &fakeClicker{}, &fakeDisplay{})
	_ = c.SelectSong(1)
	if err := c.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := c.Start(); err == nil {
		t.Fatal("expected the second Start (double-tap, same song) to be rejected")
	}
}

func TestSelectWhileRunningIsAllowedAndStartTransitions(t *testing.T) {
	clicker := &fakeClicker{}
	display := &fakeDisplay{}
	c := New(testCatalog(), clicker, display)

	_ = c.SelectSong(1)
	_ = c.Start()

	// Queue up song 2 while song 1 is still playing: must not error, and
	// must not touch what's actually playing yet.
	if err := c.SelectSong(2); err != nil {
		t.Fatalf("SelectSong while running: %v", err)
	}
	mid := c.GetState()
	if mid.PlayingSongID != 1 || mid.SelectedSongID != 2 {
		t.Fatalf("expected playing=1, selected=2 before Start; got %+v", mid)
	}
	if clicker.starts != 1 || clicker.stops != 0 {
		t.Fatalf("selecting alone must not touch the clicker; got %+v", clicker)
	}

	// Start now transitions: stop song 1's click, start song 2's.
	if err := c.Start(); err != nil {
		t.Fatalf("Start transition: %v", err)
	}
	if clicker.starts != 2 || clicker.stops != 1 || clicker.lastTempo != 112 {
		t.Fatalf("expected one stop and a second start at 112 BPM; got %+v", clicker)
	}
	final := c.GetState()
	if final.PlayingSongID != 2 {
		t.Fatalf("expected song 2 to be playing; got %+v", final)
	}
	if len(display.shown) != 2 || display.shown[1] != 2 {
		t.Fatalf("expected display.Show(2) on transition; got %v", display.shown)
	}
}

func TestMuteStopsAndResumesClickWithoutStoppingPlayback(t *testing.T) {
	clicker := &fakeClicker{}
	c := New(testCatalog(), clicker, &fakeDisplay{})
	_ = c.SelectSong(1)
	_ = c.Start()

	if err := c.SetMuted(true); err != nil {
		t.Fatalf("SetMuted(true): %v", err)
	}
	if clicker.running {
		t.Fatal("expected the clicker to stop when muted")
	}
	if !c.GetState().Running {
		t.Fatal("expected playback to still be Running while muted")
	}

	if err := c.SetMuted(false); err != nil {
		t.Fatalf("SetMuted(false): %v", err)
	}
	if !clicker.running {
		t.Fatal("expected the clicker to resume when unmuted")
	}
}

func TestDefaultEntryIsAlwaysPresent(t *testing.T) {
	c := New(testCatalog(), &fakeClicker{}, &fakeDisplay{})
	found := false
	for _, s := range c.Songs() {
		if s.ID == slideshow.DefaultID && s.Title == "Default" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a synthetic Default entry in the catalog")
	}
}

// Default's id is 0 — the same zero value SelectedSongID/PlayingSongID have
// before anything is ever selected/playing. HasSelection and Running must
// disambiguate the two: a client checking "truthy id" instead would see
// Default as indistinguishable from "nothing selected" and never enable
// Start.
func TestSelectingDefaultIsDistinguishableFromNoSelection(t *testing.T) {
	c := New(testCatalog(), &fakeClicker{}, &fakeDisplay{})

	before := c.GetState()
	if before.HasSelection {
		t.Fatal("expected HasSelection=false before any SelectSong call")
	}

	if err := c.SelectSong(slideshow.DefaultID); err != nil {
		t.Fatalf("SelectSong(DefaultID): %v", err)
	}
	after := c.GetState()
	if !after.HasSelection {
		t.Fatal("expected HasSelection=true once Default is selected, even though its id is 0")
	}
	if after.Running {
		t.Fatal("expected Running=false: selecting Default must not start it")
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Start on Default: %v", err)
	}
	if !c.GetState().Running {
		t.Fatal("expected Running=true after starting Default")
	}
}

func TestSetDefaultTempo(t *testing.T) {
	clicker := &fakeClicker{}
	c := New(testCatalog(), clicker, &fakeDisplay{})

	if err := c.SetDefaultTempo(90); err != nil {
		t.Fatalf("SetDefaultTempo: %v", err)
	}
	if err := c.SelectSong(slideshow.DefaultID); err != nil {
		t.Fatalf("SelectSong(DefaultID): %v", err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if clicker.lastTempo != 90 {
		t.Fatalf("expected the click to start at the custom Default tempo 90, got %d", clicker.lastTempo)
	}

	// Changing it again while Default is playing should restart the click
	// at the new tempo live, not just store the value for next time.
	if err := c.SetDefaultTempo(140); err != nil {
		t.Fatalf("SetDefaultTempo (live): %v", err)
	}
	if clicker.lastTempo != 140 || clicker.starts != 2 || clicker.stops != 1 {
		t.Fatalf("expected a live restart at 140 BPM, got %+v", clicker)
	}

	if err := c.SetDefaultTempo(0); err == nil {
		t.Fatal("expected an error for a non-positive tempo")
	}
}

func TestToggleLogo(t *testing.T) {
	display := &fakeDisplay{}
	c := New(testCatalog(), &fakeClicker{}, display)
	_ = c.SelectSong(1)
	_ = c.Start()

	if err := c.ToggleLogo(); err != nil {
		t.Fatalf("ToggleLogo (on): %v", err)
	}
	if !c.GetState().LogoActive {
		t.Fatal("expected LogoActive=true")
	}
	if got := display.shown[len(display.shown)-1]; got != slideshow.LogoID {
		t.Fatalf("expected display.Show(LogoID), got %d", got)
	}

	if err := c.ToggleLogo(); err != nil {
		t.Fatalf("ToggleLogo (off): %v", err)
	}
	if c.GetState().LogoActive {
		t.Fatal("expected LogoActive=false")
	}
	if got := display.shown[len(display.shown)-1]; got != 1 {
		t.Fatalf("expected display.Show back to the playing song (1), got %d", got)
	}
}

func TestToggleLogoResumesDefaultWhenNothingPlaying(t *testing.T) {
	display := &fakeDisplay{}
	c := New(testCatalog(), &fakeClicker{}, display)

	_ = c.ToggleLogo()
	_ = c.ToggleLogo()

	if got := display.shown[len(display.shown)-1]; got != slideshow.DefaultID {
		t.Fatalf("expected display.Show(DefaultID) when nothing was playing, got %d", got)
	}
}
