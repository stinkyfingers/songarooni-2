// Package state owns playback state and coordinates song selection, the
// click engine, and the slide display. All of it is protected by a single
// mutex: HTTP requests, the display's background goroutine, and the click
// engine must never race against each other.
package state

import (
	"fmt"
	"sync"

	"songarooni/internal/click"
	"songarooni/internal/slideshow"
	"songarooni/internal/songs"
)

// Display shows a song's slides. slideshow.Slideshow satisfies this.
type Display interface {
	Show(id int)
}

// defaultTempo is the Default entry's starting tempo, before anyone sets
// one via SetDefaultTempo.
const defaultTempo = 120

// State is a snapshot of current playback state, safe to copy and return
// from an API handler.
//
// Selected and Playing are deliberately separate: Selected is whatever the
// phone most recently chose (what Start will act on next); Playing is
// whichever song's click/slides are actually live right now. They differ
// whenever a song is queued up while another one is still playing.
type State struct {
	// HasSelection distinguishes "nothing selected" from "Default (id 0) is
	// selected" — SelectedSongID alone can't, since both read as 0.
	HasSelection   bool   `json:"hasSelection"`
	SelectedSongID int    `json:"selectedSongId"`
	SelectedTitle  string `json:"selectedTitle"`
	SelectedTempo  int    `json:"selectedTempo"`

	// Running is the Playing equivalent of HasSelection, for the same
	// reason: PlayingSongID alone can't distinguish "nothing playing" from
	// "Default is playing".
	PlayingSongID int    `json:"playingSongId"`
	PlayingTitle  string `json:"playingTitle"`
	PlayingTempo  int    `json:"playingTempo"`

	Running    bool   `json:"running"` // PlayingSongID != 0
	Muted      bool   `json:"muted"`
	LogoActive bool   `json:"logoActive"`
	Error      string `json:"error,omitempty"`
}

// Controller is the single source of truth for playback state.
type Controller struct {
	mu sync.Mutex

	catalog     []*songs.Song
	byID        map[int]*songs.Song
	defaultSong *songs.Song // same pointer as byID[slideshow.DefaultID]

	selected   *songs.Song
	playing    *songs.Song
	muted      bool
	logoActive bool
	lastErr    string

	clicker click.Clicker
	display Display
}

// New builds a Controller over catalog, driving clicker and display. A
// synthetic "Default" entry (id slideshow.DefaultID) is always added, for
// clicking at a manually-set tempo on songs that aren't in the catalog —
// songs.LoadFile never produces id 0 itself (ids must be positive), so this
// can't collide with a real song.
func New(catalog []songs.Song, clicker click.Clicker, display Display) *Controller {
	defaultSong := &songs.Song{
		ID:     slideshow.DefaultID,
		Title:  "Default",
		Slides: "default",
		Tempo:  defaultTempo,
	}

	all := make([]*songs.Song, 0, len(catalog)+1)
	all = append(all, defaultSong)
	byID := map[int]*songs.Song{defaultSong.ID: defaultSong}
	for i := range catalog {
		s := &catalog[i]
		all = append(all, s)
		byID[s.ID] = s
	}

	return &Controller{
		catalog:     all,
		byID:        byID,
		defaultSong: defaultSong,
		clicker:     clicker,
		display:     display,
	}
}

// Songs returns the song catalog (including the synthetic Default entry)
// in songs.csv order, Default first.
func (c *Controller) Songs() []songs.Song {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]songs.Song, len(c.catalog))
	for i, s := range c.catalog {
		out[i] = *s
	}
	return out
}

// SelectSong chooses id as the selected song — what Start will act on next.
// Always allowed, even while another song is playing, so the drummer can
// queue up the next number before the current one ends.
func (c *Controller) SelectSong(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	song, ok := c.byID[id]
	if !ok {
		return fmt.Errorf("no song with id %d", id)
	}
	c.selected = song
	c.lastErr = ""
	return nil
}

// Start makes the selected song the one playing: its click (unless muted)
// and its slide display. If a different song is already playing, that
// song's click is stopped first — a live transition, not an error. Calling
// Start again for the song that's already playing (a double-tap) is
// rejected; so is calling it with nothing selected.
func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.selected == nil {
		return fmt.Errorf("no song selected")
	}
	if c.playing != nil && c.playing.ID == c.selected.ID {
		return fmt.Errorf("already running")
	}

	if c.playing != nil && !c.muted {
		if err := c.clicker.Stop(); err != nil {
			c.lastErr = err.Error()
			return err
		}
	}
	if !c.muted {
		if err := c.clicker.Start(c.selected.Tempo); err != nil {
			c.lastErr = err.Error()
			return err
		}
	}
	c.display.Show(c.selected.ID)
	c.playing = c.selected
	c.lastErr = ""
	return nil
}

// Stop stops the click immediately and leaves the slides visible. The
// selection is untouched, so the phone doesn't lose it after a stop.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.playing == nil {
		return nil
	}
	c.playing = nil

	var err error
	if !c.muted {
		err = c.clicker.Stop()
	}
	if err != nil {
		c.lastErr = err.Error()
		return err
	}
	return nil
}

// SetMuted toggles whether the click is audible. If a song is currently
// playing, the click engine is started or stopped immediately to match;
// otherwise the preference just takes effect on the next Start.
func (c *Controller) SetMuted(muted bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if muted == c.muted {
		return nil
	}
	c.muted = muted
	if c.playing == nil {
		return nil
	}

	var err error
	if muted {
		err = c.clicker.Stop()
	} else {
		err = c.clicker.Start(c.playing.Tempo)
	}
	if err != nil {
		c.lastErr = err.Error()
		return err
	}
	return nil
}

// SetDefaultTempo sets the Default entry's tempo (unlike every real song's,
// which stays fixed from songs.csv). If Default is the song currently
// playing, the click is restarted at the new tempo immediately.
func (c *Controller) SetDefaultTempo(bpm int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if bpm <= 0 {
		return fmt.Errorf("tempo must be positive")
	}
	c.defaultSong.Tempo = bpm

	if c.playing != c.defaultSong || c.muted {
		return nil
	}
	if err := c.clicker.Stop(); err != nil {
		c.lastErr = err.Error()
		return err
	}
	if err := c.clicker.Start(bpm); err != nil {
		c.lastErr = err.Error()
		return err
	}
	return nil
}

// ToggleLogo pauses whatever slides are showing and displays the logo
// full-screen, or — called again — resumes whatever should be showing
// (the playing song, or the Default deck if nothing is playing). The click
// is untouched either way; this only ever affects the display.
func (c *Controller) ToggleLogo() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.logoActive {
		c.logoActive = false
		if c.playing != nil {
			c.display.Show(c.playing.ID)
		} else {
			c.display.Show(slideshow.DefaultID)
		}
		return nil
	}
	c.logoActive = true
	c.display.Show(slideshow.LogoID)
	return nil
}

// GetState returns a snapshot of the current state.
func (c *Controller) GetState() State {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := State{Muted: c.muted, LogoActive: c.logoActive, Error: c.lastErr}
	if c.selected != nil {
		s.HasSelection = true
		s.SelectedSongID = c.selected.ID
		s.SelectedTitle = c.selected.Title
		s.SelectedTempo = c.selected.Tempo
	}
	if c.playing != nil {
		s.PlayingSongID = c.playing.ID
		s.PlayingTitle = c.playing.Title
		s.PlayingTempo = c.playing.Tempo
		s.Running = true
	}
	return s
}
