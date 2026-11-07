// go-chi/chi: a thin router on top of net/http.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"github.com/mochavin/go-framework-bench/internal/app"
)

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	render.Status(r, status)
	render.JSON(w, r, v)
}

func main() {
	a := app.New("chi")
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, 200, a.Health())
	})
	r.Get("/api/posts", func(w http.ResponseWriter, r *http.Request) {
		posts, err := a.LatestPosts(r.Context())
		if err != nil {
			writeJSON(w, r, 500, app.ServerErrorBody)
			return
		}
		writeJSON(w, r, 200, app.ListBody(posts))
	})
	r.Get("/api/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeJSON(w, r, 404, app.NotFoundBody)
			return
		}
		p, err := a.FindPost(r.Context(), id)
		switch {
		case errors.Is(err, app.ErrNotFound):
			writeJSON(w, r, 404, app.NotFoundBody)
		case err != nil:
			writeJSON(w, r, 500, app.ServerErrorBody)
		default:
			writeJSON(w, r, 200, app.PostBody(p))
		}
	})
	r.Post("/api/posts", func(w http.ResponseWriter, r *http.Request) {
		var in app.PostInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, r, 400, app.InvalidJSONBody)
			return
		}
		if errs := app.Validate(in); errs != nil {
			writeJSON(w, r, 422, errs)
			return
		}
		p, err := a.CreatePost(r.Context(), *in.Title, *in.Body)
		if err != nil {
			writeJSON(w, r, 500, app.ServerErrorBody)
			return
		}
		writeJSON(w, r, 201, app.PostBody(p))
	})
	r.Get("/api/heavy", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, 200, app.Heavy())
	})

	log.Printf("chi listening on %s", a.Addr)
	log.Fatal(http.ListenAndServe(a.Addr, r))
}
