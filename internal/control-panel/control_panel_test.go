package control_panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"songarooni/internal/camera"
	"songarooni/internal/config"
	"songarooni/internal/songs"
	"songarooni/internal/state"
)

type fakeClicker struct{}

func (fakeClicker) Start(tempo int) error { return nil }
func (fakeClicker) Stop() error           { return nil }

type fakeDisplay struct {
	liveErr error
}

func (fakeDisplay) Show(id int)                      {}
func (fakeDisplay) SetCustomIDs(ids state.CustomIDs) {}
func (f fakeDisplay) ShowLive() error                { return f.liveErr }

func testHandler() http.Handler {
	return testHandlerWithConfig(config.Config{})
}

func testHandlerWithConfig(cfg config.Config) http.Handler {
	h, _ := testHandlerWithCamera(cfg)
	return h
}

func testHandlerWithCamera(cfg config.Config) (http.Handler, *camera.Proxy) {
	catalog := []songs.Song{{ID: 1, Title: "Song One", Slides: "song-one", Tempo: 120}}
	controller := state.New(catalog, fakeClicker{}, fakeDisplay{})
	cameraProxy := camera.New(cfg.CameraIP, cfg.CameraPort, cfg.CameraUsername, cfg.CameraPassword)
	return apiHandler(controller, cfg, cameraProxy), cameraProxy
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

func TestAPILiveSucceeds(t *testing.T) {
	h := testHandler()

	req := httptest.NewRequest("POST", "/api/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got state.State
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !got.LiveActive {
		t.Fatal("expected liveActive=true in the response")
	}
}

func TestAPILiveFailsWhenNoCameraConfigured(t *testing.T) {
	catalog := []songs.Song{{ID: 1, Title: "Song One", Slides: "song-one", Tempo: 120}}
	controller := state.New(catalog, fakeClicker{}, fakeDisplay{liveErr: errors.New("no camera configured")})
	cameraProxy := camera.New("", 0, "", "")
	h := apiHandler(controller, config.Config{}, cameraProxy)

	req := httptest.NewRequest("POST", "/api/live", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPICameraConfigGetReturnsCurrentTarget(t *testing.T) {
	h, _ := testHandlerWithCamera(config.Config{CameraIP: "192.168.1.42", CameraPort: 8080})

	req := httptest.NewRequest("GET", "/api/camera-config", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got.IP != "192.168.1.42" || got.Port != 8080 {
		t.Fatalf("expected the configured target, got %+v", got)
	}
}

func TestAPICameraConfigPostUpdatesTheRunningProxy(t *testing.T) {
	h, cameraProxy := testHandlerWithCamera(config.Config{})

	body, _ := json.Marshal(map[string]any{"ip": "192.168.1.99", "port": 9090})
	req := httptest.NewRequest("POST", "/api/camera-config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ip, port := cameraProxy.Target()
	if ip != "192.168.1.99" || port != 9090 {
		t.Fatalf("expected the proxy's target to be updated, got %s:%d", ip, port)
	}
}

func TestAPICameraConfigPostRejectsInvalidPort(t *testing.T) {
	h, _ := testHandlerWithCamera(config.Config{})

	body, _ := json.Marshal(map[string]any{"ip": "192.168.1.99", "port": 0})
	req := httptest.NewRequest("POST", "/api/camera-config", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-positive port with a non-empty IP, got %d", w.Code)
	}
}

func TestAPICameraDiscoverReturnsFoundHosts(t *testing.T) {
	h, _ := testHandlerWithCamera(config.Config{CameraPort: 8080})

	req := httptest.NewRequest("POST", "/api/camera/discover", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// No real camera on this machine's LAN during a test run — just
	// confirming the endpoint responds with a (possibly empty) found list
	// rather than erroring, since Discover's actual scanning is already
	// covered directly in internal/camera's own tests.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Found []string `json:"found"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
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
