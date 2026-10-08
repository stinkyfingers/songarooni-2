// Package httpapi exposes the Controller over the JSON API the phone
// (and, incidentally, anything else on the network) talks to. No
// authentication: an accepted risk for a private band network (see
// initial-plan.md).
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"songarooni/internal/state"
)

type songDTO struct {
	ID    int               `json:"id"`
	Title string            `json:"title"`
	Tempo int               `json:"tempo"`
	Extra map[string]string `json:"extra,omitempty"`
}

type fieldsDTO struct {
	Extra     []string `json:"extra"`     // every songs.csv column beyond id/title/slides/tempo
	Displayed []string `json:"displayed"` // the subset config.yaml's displayed_fields selects for the list view
}

// Handler builds the /api/* mux for controller. extraFields is every
// songs.csv column beyond the required four, in header order; displayedFields
// is the config.yaml-selected subset to show in the phone's list view.
func Handler(controller *state.Controller, extraFields, displayedFields []string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/songs", func(w http.ResponseWriter, r *http.Request) {
		catalog := controller.Songs()
		dtos := make([]songDTO, len(catalog))
		for i, s := range catalog {
			dtos[i] = songDTO{ID: s.ID, Title: s.Title, Tempo: s.Tempo, Extra: s.Extra}
		}
		writeJSON(w, http.StatusOK, dtos)
	})

	mux.HandleFunc("GET /api/fields", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, fieldsDTO{Extra: extraFields, Displayed: displayedFields})
	})

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, controller.GetState())
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
		log.Printf("httpapi: encoding response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
