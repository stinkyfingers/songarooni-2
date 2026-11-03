package control_panel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"songarooni/internal/songs"
	"songarooni/internal/state"
)

type fakeClicker struct{}

func (fakeClicker) Start(tempo int) error { return nil }
func (fakeClicker) Stop() error           { return nil }

type fakeDisplay struct{}

func (fakeDisplay) Show(id int) {}

func testHandler() http.Handler {
	catalog := []songs.Song{{ID: 1, Title: "Song One", Slides: "song-one", Tempo: 120}}
	controller := state.New(catalog, fakeClicker{}, fakeDisplay{})
	return apiHandler(controller, nil, nil)
}

// The slideshow kiosk page polls these two endpoints cross-origin (it runs
// on its own port — see plans/browser-slideshow-plan.md), which needs this
// header or the browser blocks the response from JS entirely.
func TestStateAndSongsAllowCrossOrigin(t *testing.T) {
	h := testHandler()

	for _, path := range []string{"/api/state", "/api/songs"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: expected Access-Control-Allow-Origin: *, got %q", path, got)
		}
	}
}

// The CORS header is scoped deliberately — not applied blanket across
// every route — so confirm an endpoint that doesn't need it doesn't carry
// it either, catching an accidental "just allow everything" regression.
func TestFieldsDoesNotAllowCrossOrigin(t *testing.T) {
	h := testHandler()

	req := httptest.NewRequest("GET", "/api/fields", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("/api/fields: expected no Access-Control-Allow-Origin header, got %q", got)
	}
}
