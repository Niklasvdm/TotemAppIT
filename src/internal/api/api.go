// Package api is the HTTP transport: it maps requests to Catalog calls and
// domain values to JSON DTOs. It knows nothing about SQL or encryption.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

// Catalog is the slice of the data layer the API consumes. *store.Store
// satisfies it; tests supply a fake. (Interface defined by the consumer.)
type Catalog interface {
	ListAnimals(ctx context.Context, f store.Filter) ([]store.Animal, error)
	GetAnimal(ctx context.Context, slug, lang string) (*store.AnimalDetail, error)
	Similar(ctx context.Context, slug, lang string, limit int) ([]store.Animal, error)
	ListTraits(ctx context.Context, lang string) ([]store.Trait, error)
}

// Server holds the router and its dependencies.
type Server struct {
	cat    Catalog
	Router http.Handler
}

// New wires the routes and middleware.
func New(cat Catalog) *Server {
	s := &Server{cat: cat}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/animals", s.listAnimals)
		r.Get("/animals/{slug}", s.getAnimal)
		r.Get("/animals/{slug}/similar", s.similar)
		r.Get("/traits", s.listTraits)
	})

	s.Router = r
	return s
}

// --- DTOs (JSON shapes; separate from the domain types) ---------------------

type animalDTO struct {
	Slug   string   `json:"slug"`
	Name   string   `json:"name"`
	Traits []string `json:"traits"`
}

type animalDetailDTO struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	NameNL      string   `json:"nameNl"`
	AltNames    string   `json:"altNames,omitempty"`
	Description string   `json:"description"`
	Traits      []string `json:"traits"`
	SourceURL   string   `json:"sourceUrl,omitempty"`
}

type traitDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// --- Handlers ---------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) listAnimals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.Filter{
		Lang:    validLang(q.Get("lang")),
		Query:   q.Get("q"),
		Include: splitCSV(q.Get("include")),
		Exclude: splitCSV(q.Get("exclude")),
	}

	animals, err := s.cat.ListAnimals(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list animals")
		return
	}

	out := make([]animalDTO, 0, len(animals))
	for _, a := range animals {
		out = append(out, animalDTO{Slug: a.Slug, Name: a.Name, Traits: a.Traits})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getAnimal(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	lang := validLang(r.URL.Query().Get("lang"))

	d, err := s.cat.GetAnimal(r.Context(), slug, lang)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "animal not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load animal")
		return
	}

	writeJSON(w, http.StatusOK, animalDetailDTO{
		Slug:        d.Slug,
		Name:        d.Name,
		NameNL:      d.NameNL,
		AltNames:    d.AltNames,
		Description: d.Description,
		Traits:      d.Traits,
		SourceURL:   d.SourceURL,
	})
}

func (s *Server) similar(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	lang := validLang(r.URL.Query().Get("lang"))
	limit := 10
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 50 {
		limit = n
	}

	animals, err := s.cat.Similar(r.Context(), slug, lang, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not compute similar animals")
		return
	}

	out := make([]animalDTO, 0, len(animals))
	for _, a := range animals {
		out = append(out, animalDTO{Slug: a.Slug, Name: a.Name, Traits: a.Traits})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listTraits(w http.ResponseWriter, r *http.Request) {
	lang := validLang(r.URL.Query().Get("lang"))

	traits, err := s.cat.ListTraits(r.Context(), lang)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list traits")
		return
	}

	out := make([]traitDTO, 0, len(traits))
	for _, t := range traits {
		out = append(out, traitDTO{Key: t.Key, Label: t.Label, Count: t.Count})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- helpers ----------------------------------------------------------------

// validLang clamps the lang param to a supported language, defaulting to it.
func validLang(q string) string {
	switch q {
	case "it", "en", "nl":
		return q
	default:
		return "it"
	}
}

// splitCSV splits a comma list into trimmed, non-empty values (nil when empty).
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
