package slideshow

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleImagesServesFromParentDir(t *testing.T) {
	parentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parentDir, "song-one"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentDir, "song-one", "1.png"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(":0", parentDir, nil, 0, 0, 0, 0, "8080")
	req := httptest.NewRequest("GET", "/images/song-one/1.png", nil)
	w := httptest.NewRecorder()
	s.handleImages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "fake" {
		t.Fatalf("expected the file's contents, got %q", w.Body.String())
	}
}

func TestHandleLiveCameraFailsWithNoProxySet(t *testing.T) {
	s := New(":0", t.TempDir(), nil, 0, 0, 0, 0, "8080")
	req := httptest.NewRequest("GET", "/live-camera", nil)
	w := httptest.NewRecorder()
	s.handleLiveCamera(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleLiveCameraDelegatesToTheConfiguredProxy(t *testing.T) {
	s := New(":0", t.TempDir(), nil, 0, 0, 0, 0, "8080")
	s.SetCameraProxy(fakeCameraProxy{configured: true, streamBody: "fake mjpeg bytes"})

	req := httptest.NewRequest("GET", "/live-camera", nil)
	w := httptest.NewRecorder()
	s.handleLiveCamera(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "fake mjpeg bytes" {
		t.Fatalf("expected the proxy's body to pass through, got %q", w.Body.String())
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
