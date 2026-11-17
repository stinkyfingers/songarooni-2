package slideshow

import (
	"context"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"songarooni/internal/slides"
	"songarooni/internal/songs"
)

// crossfadePadding is added to every slide's dwell time, matching the
// client's own crossfade transition length — without it the next slide
// would start loading/revealing before the current one finishes fading
// out.
const crossfadePadding = 2 * time.Second

// Show implements state.Display: it resolves id's slide deck and starts
// cycling it, pushing each slide's URL to every connected SSE client (see
// sseHandler) once per slide_interval_seconds. It returns immediately — the
// cycling itself runs in its own goroutine — so Controller.Start/ToggleLogo
// can call it while still holding their own lock without deadlocking (the
// previous implementation looped forever inline here, which hung the whole
// app, Ctrl-C included, the moment a song started). Calling Show again (a
// new song, Default, or the logo) cancels whatever deck was previously
// cycling; Run's ctx being done (app shutdown) cancels it too.
func (s *Slideshow) Show(id int) {
	deck, err := slides.Resolve(s.parentDir, s.songMap, s.logoEvery, s.commercialEvery, id)
	if err != nil {
		log.Printf("display: error resolving slides for song id %d: %v", id, err)
		return
	}

	s.showMu.Lock()
	if s.cancelShow != nil {
		s.cancelShow()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancelShow = cancel
	s.showMu.Unlock()

	go s.cycle(ctx, deck)
}

// cycle pushes deck's slides to stateChan one at a time, holding each on
// screen for its own dwell duration (see dwell) before moving to the next,
// until ctx is done.
func (s *Slideshow) cycle(ctx context.Context, deck []songs.Data) {
	i := 0
	for {
		data := deck[i]
		u, err := url.JoinPath("/images", data.Path)
		if err != nil {
			log.Printf("display: error joining path for slide %s: %v", data, err)
			return
		}

		select {
		case s.stateChan <- u:
		case <-ctx.Done():
			return
		}

		select {
		case <-time.After(data.Length + crossfadePadding):
		case <-ctx.Done():
			return
		}
		i = (i + 1) % len(deck)
	}
}

// dwell is how long slide (a path relative to s.parentDir) should stay on
// screen: an .mp4's own duration, or the configured slide_interval_seconds
// for everything else — including an .mp4 whose duration can't be read, so
// a malformed video file never stalls the whole deck.
func (s *Slideshow) dwell(slide string) time.Duration {
	if strings.HasSuffix(strings.ToLower(slide), ".mp4") {
		d, err := songs.MP4Duration(filepath.Join(s.parentDir, slide))
		if err == nil {
			return d + crossfadePadding
		}
		log.Printf("display: reading %s duration: %v; falling back to the configured interval", slide, err)
	}
	return time.Duration(s.intervalMS)*time.Millisecond + crossfadePadding
}
