// Package control_panel serves the phone's control page and its JSON API
// — song selection, start/stop, tempo, mute, logo toggle. No
// authentication: an accepted risk for a private band network (see
// initial-plan.md).
package control_panel

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"songarooni/internal/config"
	"songarooni/internal/state"
)

//go:embed web
var rawAssets embed.FS

// webAssets is the embedded web/ directory, rooted so that index.html,
// app.js and styles.css sit at its top level — matching the absolute paths
// (/styles.css, /app.js) that index.html itself references.
var webAssets = mustSub(rawAssets, "web")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// Run builds the control panel's mux (the JSON API plus the embedded phone
// page) and serves it on addr until ctx is done.
func Run(ctx context.Context, controller *state.Controller, configuration config.Config) error {
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler(controller, configuration))
	mux.Handle("/", noCache(http.FileServerFS(webAssets)))

	server := &http.Server{Addr: configuration.Addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("control panel listening on %s", configuration.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func apiHandler(controller *state.Controller, configuration config.Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/songs", func(w http.ResponseWriter, r *http.Request) {
		allowCrossOrigin(w) // the slideshow kiosk page, on its own port, polls this
		catalog := controller.Songs()
		writeJSON(w, http.StatusOK, catalog)
	})

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		allowCrossOrigin(w) // the slideshow kiosk page, on its own port, polls this
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, configuration)
	})

	mux.HandleFunc("POST /api/select", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID int `json:"id"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := controller.SelectSong(body.ID); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/select/next", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.SelectNext(); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/select/previous", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.SelectPrevious(); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/select/default", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.StartDefault(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.Start(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.Stop(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/mute", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Muted bool `json:"muted"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := controller.SetMuted(body.Muted); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/tempo", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BPM int `json:"bpm"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := controller.SetDefaultTempo(body.BPM); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	mux.HandleFunc("POST /api/logo", func(w http.ResponseWriter, r *http.Request) {
		if err := controller.ToggleLogo(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, controller.GetState())
	})

	return mux
}

// statusFor picks 404 for an unknown song id and 400 for everything else
// SelectSong can return, matching Phase 6's "invalid song id -> 400/404"
// requirement.
func statusFor(err error) int {
	if err == nil {
		return http.StatusOK
	}
	const notFoundPrefix = "no song with id"
	msg := err.Error()
	if len(msg) >= len(notFoundPrefix) && msg[:len(notFoundPrefix)] == notFoundPrefix {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("control-panel: encoding response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// allowCrossOrigin permits any origin to read this response. Scoped to just
// the couple of read-only GET endpoints the slideshow kiosk page (served
// from its own port — a different origin) needs to poll — not applied
// blanket across the whole API — since this is a private, unauthenticated
// local network already (see the package doc) there's nothing a
// cross-origin read here exposes that a direct request wouldn't already.
func allowCrossOrigin(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// noCache stops the browser from caching the control page's static assets.
// This is a small, locally-served app under active development — serving a
// stale cached app.js against a server whose API shape has since changed is
// a worse failure mode than re-fetching a few KB on every load.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
