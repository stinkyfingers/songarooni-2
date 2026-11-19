package slideshow

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"time"

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
	deck, err := s.Resolve(id)
	if err != nil {
		log.Printf("display: error resolving slides for song id %d: %v", id, err)
		return
	}
	commercials := s.resolveOptional(s.customIDs.CommercialID)
	logos := s.resolveOptional(s.customIDs.LogoID)
	candid := s.resolveOptional(s.customIDs.CandidID)

	s.showMu.Lock()
	if s.cancelShow != nil {
		s.cancelShow()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cancelShow = cancel
	s.showMu.Unlock()

	go s.cycle(ctx, id, deck, commercials, logos, candid)
}

// ShowLive implements state.Display: it cancels whatever deck is currently
// cycling (same as Show, but with nothing new to start in its place) and
// pushes the flat "live" sentinel to every connected SSE client, which
// app.js recognizes and points its own <img> at this server's
// /live-camera route instead of rendering a normal slide. Unlike Show,
// there's no new cycle goroutine and no dwell timer — Live stays up until
// something else explicitly replaces it (a song, the logo, or another
// ShowLive call). Fails only if no camera has ever been configured at
// all (see SetCameraProxy) — deliberately not a live reachability check:
// that would mean a network round-trip while Controller still holds its
// lock (see GoLive), and a configured-but-unreachable camera is already
// handled visibly on the kiosk side (the <img>'s error/retry path).
func (s *Slideshow) ShowLive() error {
	if s.camera == nil || !s.camera.Configured() {
		return fmt.Errorf("no camera configured")
	}

	s.showMu.Lock()
	if s.cancelShow != nil {
		s.cancelShow()
		s.cancelShow = nil
	}
	s.showMu.Unlock()

	go func() {
		select {
		case s.stateChan <- "live":
		case <-s.ctx.Done():
		}
	}()
	return nil
}

// cycle pushes deck's slides to stateChan one at a time, holding each on
// screen for its own dwell duration (see dwell) before moving to the next,
// until ctx is done.
func (s *Slideshow) cycle(ctx context.Context, id int, deck, commercials, logos, candid []songs.Data) {
	commercialIndex := 0
	logoIndex := 0
	candidIndex := 0
	i := 0
	advanceIndex := func() {
		i = (i + 1) % len(deck)
	}
	for {
		data := deck[i]
		if !s.displayData(ctx, data) {
			return // ctx was canceled (or the path was unjoinable) — stop the goroutine, same as the old inline return did
		}
		// no extras on logos
		if id == s.customIDs.LogoID {
			advanceIndex()
			continue
		}

		// s.logoEvery/commercialEvery/candidEvery <= 0 means "disabled"
		// (see config.go's doc comments) — checked before the modulo, not
		// just via len(...) > 0, since i % 0 panics regardless of whether
		// the deck is empty.
		if s.logoEvery > 0 && i%s.logoEvery == s.logoEvery-1 && len(logos) > 0 {
			if !s.displayData(ctx, logos[logoIndex]) {
				return
			}
			if logoIndex++; logoIndex >= len(logos) {
				logoIndex = 0
			}
		}
		// only logos are exempt from commercials
		if id == s.customIDs.CommercialID {
			advanceIndex()
			continue
		}

		if s.commercialEvery > 0 && i%s.commercialEvery == s.commercialEvery-1 && len(commercials) > 0 {
			if !s.displayData(ctx, commercials[commercialIndex]) {
				return
			}
			if commercialIndex++; commercialIndex >= len(commercials) {
				commercialIndex = 0
			}
		}
		// candids are exempt from candids
		if id == s.customIDs.CandidID {
			advanceIndex()
			continue
		}
		if s.candidEvery > 0 && i%s.candidEvery == s.candidEvery-1 && len(candid) > 0 {
			if !s.displayData(ctx, candid[candidIndex]) {
				return
			}
			if candidIndex++; candidIndex >= len(candid) {
				candidIndex = 0
			}
		}
		advanceIndex()
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

// Resolve picks id's slide deck.
func (s *Slideshow) Resolve(id int) ([]songs.Data, error) {
	songDir, ok := s.songMap[id]
	if !ok {
		log.Println("slides: song id", id, "not found in songMap")
		return []songs.Data{{Path: ""}}, nil
	}
	return songs.ValidateSlideDir(s.parentDir, songDir.Slides, s.defaultLength)
}

// resolveOptional is Resolve, but for a category that's allowed to be
// legitimately empty — Commercials, Logo, and (once wired in) Candid.
// Candid in particular starts every show with nothing uploaded yet, so a
// missing/empty directory there just means "nothing to interleave yet,"
// not a reason to abort the whole display the way an error from the main
// song's own deck should.
func (s *Slideshow) resolveOptional(id int) []songs.Data {
	data, err := s.Resolve(id)
	if err != nil {
		log.Printf("display: %v (treating as empty)", err)
		return nil
	}
	return data
}
