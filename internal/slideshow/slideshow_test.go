package slideshow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"songarooni/internal/slides"
)

func writeImage(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
}

func testSlideshow(t *testing.T) (*Slideshow, string, string) {
	t.Helper()
	root := t.TempDir()
	parentDir := filepath.Join(root, "songs")
	defaultDir := filepath.Join(root, "default")
	writeImage(t, filepath.Join(parentDir, "song-one"), "1.png")
	writeImage(t, defaultDir, "d.png")

	songMap := map[int]string{1: "song-one"}
	s := New(":0", parentDir, defaultDir, songMap, 0, 20, "8080")
	return s, parentDir, defaultDir
}

func TestHandleSlidesResolvesKnownSong(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/slides?id=1", nil)
	w := httptest.NewRecorder()
	s.handleSlides(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var urls []string
	if err := json.Unmarshal(w.Body.Bytes(), &urls); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(urls) != 1 || urls[0] != "/images/song/song-one/1.png" {
		t.Fatalf("expected [/images/song/song-one/1.png], got %v", urls)
	}
}

func TestHandleSlidesResolvesDefault(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/slides?id=0", nil)
	w := httptest.NewRecorder()
	s.handleSlides(w, req)

	var urls []string
	if err := json.Unmarshal(w.Body.Bytes(), &urls); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(urls) != 1 || urls[0] != "/images/default/d.png" {
		t.Fatalf("expected [/images/default/d.png], got %v", urls)
	}
}

func TestHandleSlidesResolvesLogo(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/slides?id=-1", nil)
	w := httptest.NewRecorder()
	s.handleSlides(w, req)

	var urls []string
	if err := json.Unmarshal(w.Body.Bytes(), &urls); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(urls) != 1 || urls[0] != "/logo.png" {
		t.Fatalf("expected [/logo.png], got %v", urls)
	}
}

func TestHandleSlidesRejectsMissingID(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/slides", nil)
	w := httptest.NewRecorder()
	s.handleSlides(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing id, got %d", w.Code)
	}
}

func TestSlideURLEscapesSpecialCharacters(t *testing.T) {
	deck := slides.Deck{Base: slides.SongRoot, Subdir: "song one"}
	got := slideURL(deck, "slide #1.png")
	want := "/images/song/song%20one/slide%20%231.png"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestHandleLogoServesEmbeddedPNG(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/logo.png", nil)
	w := httptest.NewRecorder()
	s.handleLogo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected image/png, got %q", ct)
	}
	if len(w.Body.Bytes()) != len(embeddedLogo) {
		t.Fatalf("expected the embedded logo's bytes, got %d bytes", w.Body.Len())
	}
}

func TestHandleConfigReturnsAPIPortAndInterval(t *testing.T) {
	s, _, _ := testSlideshow(t)

	req := httptest.NewRequest("GET", "/config", nil)
	w := httptest.NewRecorder()
	s.handleConfig(w, req)

	var got struct {
		APIPort    string `json:"apiPort"`
		IntervalMs int    `json:"intervalMs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.APIPort != "8080" || got.IntervalMs != 20000 {
		t.Fatalf("expected apiPort=8080 intervalMs=20000, got %+v", got)
	}
}

func TestEmbeddedIndexPageServes(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(webAssets))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %q", ct)
	}
}
