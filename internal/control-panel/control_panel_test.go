package control_panel

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"songarooni/internal/config"
	"songarooni/internal/songs"
	"songarooni/internal/state"
)

type fakeClicker struct{}

func (fakeClicker) Start(tempo int) error { return nil }
func (fakeClicker) Stop() error           { return nil }

type fakeDisplay struct{}

func (fakeDisplay) Show(id int)                    {}
func (fakeDisplay) SetCustomIDs(ids state.CustomIDs) {}

func testHandler() http.Handler {
	return testHandlerWithConfig(config.Config{})
}

func testHandlerWithConfig(cfg config.Config) http.Handler {
	catalog := []songs.Song{{ID: 1, Title: "Song One", Slides: "song-one", Tempo: 120}}
	controller := state.New(catalog, fakeClicker{}, fakeDisplay{})
	return apiHandler(controller, cfg)
}

// pngSignature is enough for http.DetectContentType to recognize
// image/png — it only matches the leading magic bytes, not a fully valid
// file, so this is all a sniffing test needs.
var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func multipartRequest(t *testing.T, field string, files map[string][]byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := w.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/candid/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
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

func TestCandidUploadSavesValidImage(t *testing.T) {
	slideDir := t.TempDir()
	h := testHandlerWithConfig(config.Config{SlideDir: slideDir})

	req := multipartRequest(t, "photos", map[string][]byte{"photo.jpg": pngSignature})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Saved  int      `json:"saved"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Saved != 1 || len(got.Errors) != 0 {
		t.Fatalf("expected 1 saved, 0 errors, got %+v", got)
	}

	entries, err := os.ReadDir(filepath.Join(slideDir, "candid"))
	if err != nil {
		t.Fatalf("reading candid dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file written to slide_dir/candid, got %d", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".png" {
		t.Fatalf("expected the sniffed .png extension (not the client's claimed .jpg), got %q", entries[0].Name())
	}
}

func TestCandidUploadRejectsUnsupportedFileType(t *testing.T) {
	slideDir := t.TempDir()
	h := testHandlerWithConfig(config.Config{SlideDir: slideDir})

	req := multipartRequest(t, "photos", map[string][]byte{"notes.txt": []byte("just some text, not an image")})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when nothing in the batch is a supported image, got %d: %s", w.Code, w.Body.String())
	}

	entries, _ := os.ReadDir(filepath.Join(slideDir, "candid"))
	if len(entries) != 0 {
		t.Fatalf("expected nothing written for an unsupported file type, got %d entries", len(entries))
	}
}

func TestCandidUploadRequiresAtLeastOneFile(t *testing.T) {
	h := testHandlerWithConfig(config.Config{SlideDir: t.TempDir()})

	req := multipartRequest(t, "photos", map[string][]byte{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a request with no files, got %d", w.Code)
	}
}

// One bad file in a multi-file batch shouldn't sink the good ones — a
// real scenario here, since a single picker trip can hand over several
// files at once.
func TestCandidUploadSavesGoodFilesDespiteOneBadOne(t *testing.T) {
	slideDir := t.TempDir()
	h := testHandlerWithConfig(config.Config{SlideDir: slideDir})

	req := multipartRequest(t, "photos", map[string][]byte{
		"good.jpg": pngSignature,
		"bad.txt":  []byte("not an image"),
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 since at least one file saved, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Saved  int      `json:"saved"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.Saved != 1 || len(got.Errors) != 1 {
		t.Fatalf("expected 1 saved and 1 reported error, got %+v", got)
	}
}
