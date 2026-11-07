// net/http from the standard library, using Go 1.22+ method/path patterns.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/mochavin/go-framework-bench/internal/app"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	a := app.New("stdlib")
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, a.Health())
	})
	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, r *http.Request) {
		posts, err := a.LatestPosts(r.Context())
		if err != nil {
			writeJSON(w, 500, app.ServerErrorBody)
			return
		}
		writeJSON(w, 200, app.ListBody(posts))
	})
	mux.HandleFunc("GET /api/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeJSON(w, 404, app.NotFoundBody)
			return
		}
		p, err := a.FindPost(r.Context(), id)
		switch {
		case errors.Is(err, app.ErrNotFound):
			writeJSON(w, 404, app.NotFoundBody)
		case err != nil:
			writeJSON(w, 500, app.ServerErrorBody)
		default:
			writeJSON(w, 200, app.PostBody(p))
		}
	})
	mux.HandleFunc("POST /api/posts", func(w http.ResponseWriter, r *http.Request) {
		var in app.PostInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, 400, app.InvalidJSONBody)
			return
		}
		if errs := app.Validate(in); errs != nil {
			writeJSON(w, 422, errs)
			return
		}
		p, err := a.CreatePost(r.Context(), *in.Title, *in.Body)
		if err != nil {
			writeJSON(w, 500, app.ServerErrorBody)
			return
		}
		writeJSON(w, 201, app.PostBody(p))
	})
	mux.HandleFunc("GET /api/heavy", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, app.Heavy())
	})

	log.Printf("stdlib listening on %s", a.Addr)
	log.Fatal(http.ListenAndServe(a.Addr, mux))
}
