package slideshow

import (
	"context"
	"log"
	"net/url"
	"time"

	"songarooni/internal/slides"
	"songarooni/internal/songs"
)

// crossfadePadding is added to every slide's dwell time, matching the
// client's own crossfade transition length — without it the next slide
// would start loading/revealing before the current one finishes fading
// out.
const crossfadePadding = 1 * time.Second

// maxRevealWait caps how long displayData waits for the browser's
// POST /advance before giving up and moving on anyway — a crashed tab, a
// load error nothing else catches, or a network drop must never freeze
// the whole show. Generous enough to cover a slow decode start on
// underpowered hardware (a Pi 3, not a desktop) without being so long a
// genuinely stuck client stalls everything for a long time.
const maxRevealWait = 10 * time.Second

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
	commercials, err := slides.Resolve(s.parentDir, s.songMap, s.logoEvery, s.commercialEvery, s.customIDs.CommercialID)
	if err != nil {
		log.Printf("display: error resolving slides for song id %d: %v", id, err)
		return
	}
	logos, err := slides.Resolve(s.parentDir, s.songMap, s.logoEvery, s.commercialEvery, s.customIDs.LogoID)
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

	go s.cycle(ctx, deck, commercials, logos)
}

// cycle pushes deck's slides to stateChan one at a time, holding each on
// screen for its own dwell duration (see dwell) before moving to the next,
// until ctx is done.
func (s *Slideshow) cycle(ctx context.Context, deck, commercials, logos []songs.Data) {
	commercialIndex := 0
	logoIndex := 0
	i := 0
	for {
		data := deck[i]
		if !s.displayData(ctx, data) {
			return // ctx was canceled (or the path was unjoinable) — stop the goroutine, same as the old inline return did
		}
		if i%s.logoEvery == s.logoEvery-1 && len(logos) > 0 {
			if !s.displayData(ctx, logos[logoIndex]) {
				return
			}
			if logoIndex++; logoIndex >= len(logos) {
				logoIndex = 0
			}
		}
		if i%s.commercialEvery == s.commercialEvery-1 && len(commercials) > 0 {
			if !s.displayData(ctx, commercials[commercialIndex]) {
				return
			}
			if commercialIndex++; commercialIndex >= len(commercials) {
				commercialIndex = 0
			}
		}
		i = (i + 1) % len(deck)
	}
}

// displayData pushes data's URL, waits for the browser to actually reveal
// it (POST /advance — see app.js's reveal() and its load-error path,
// which both call it), and only then dwells for data's own duration —
// timing the real on-screen duration from when the slide was genuinely
// shown, not from whenever it was pushed. A slow decode start on
// underpowered hardware no longer eats into the slide's own dwell time as
// a result. Returns false if ctx was canceled partway through (or the
// path itself was bad) — the caller must stop on false rather than
// looping again, since looping on an already-canceled ctx would just
// busy-spin forever instead of actually ending the goroutine.
func (s *Slideshow) displayData(ctx context.Context, data songs.Data) bool {
	u, err := url.JoinPath("/images", data.Path)
	if err != nil {
		log.Printf("display: error joining path for slide %s: %v", data, err)
		return false
	}

	select {
	case s.stateChan <- u:
	case <-ctx.Done():
		return false
	}

	// Discard any stale signal left over from a previous slide (e.g. a
	// late POST that arrived after we'd already given up waiting for it)
	// so it can't short-circuit this slide's own wait below.
	select {
	case <-s.advanceChan:
	default:
	}

	select {
	case <-s.advanceChan:
	case <-time.After(maxRevealWait):
		log.Printf("display: %s never acknowledged within %s; advancing anyway", u, maxRevealWait)
	case <-ctx.Done():
		return false
	}

	select {
	case <-time.After(data.Length + crossfadePadding):
	case <-ctx.Done():
		return false
	}

	return true
}
