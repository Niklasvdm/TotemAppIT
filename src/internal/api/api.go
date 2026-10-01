// Package api is the HTTP transport: it maps requests to Catalog calls and
// domain values to JSON DTOs. It knows nothing about SQL or encryption.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

// slugRe guards the image file path against traversal: slugs are lowercase
// letters, digits and hyphens only.
var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// Catalog is the slice of the data layer the API consumes. *store.Store
// satisfies it; tests supply a fake. (Interface defined by the consumer.)
type Catalog interface {
	ListAnimals(ctx context.Context, f store.Filter) ([]store.Animal, error)
	GetAnimal(ctx context.Context, slug, lang string) (*store.AnimalDetail, error)
	Similar(ctx context.Context, slug, lang string, limit int) ([]store.Animal, error)
	SimilarByTraits(ctx context.Context, include, exclude []string, lang string, limit int) ([]store.Animal, error)
	ListTraits(ctx context.Context, lang string) ([]store.Trait, error)
}

// Server holds the router and its dependencies.
type Server struct {
	cat      Catalog
	imageDir string            // directory of <slug>.webp files (TOTEM_IMAGE_DIR)
	Emoji    map[string]string // slug -> emoji, served at /api/v1/emoji (set by main)
	Router   http.Handler
}

// LoadEmoji reads a slug->emoji JSON map from path (returns nil on any error, so
// the frontend simply falls back to a default glyph).
func LoadEmoji(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// New wires the routes and middleware. imageDir is where animal images are served
// from (may be "" if images aren't deployed — the image route then 404s).
func New(cat Catalog, imageDir string) *Server {
	s := &Server{cat: cat, imageDir: imageDir}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	r.Get("/healthz", s.health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/animals", s.listAnimals)
		r.Get("/animals/{slug}", s.getAnimal)
		r.Get("/animals/{slug}/similar", s.similar)
		r.Get("/similar", s.similarByTraits)
		r.Get("/animals/{slug}/image", s.animalImage)
		r.Get("/traits", s.listTraits)
		r.Get("/emoji", s.emoji)
	})

	s.Router = r
	return s
}

// --- DTOs (JSON shapes; separate from the domain types) ---------------------

type animalDTO struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Traits      []string `json:"traits"`
	Score       float64  `json:"score,omitempty"` // Jaccard 0..1, only on similarity results
}

type animalDetailDTO struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	NameNL      string    `json:"nameNl"`
	AltNames    string    `json:"altNames,omitempty"`
	Description string    `json:"description"`
	Traits      []string  `json:"traits"`
	SourceURL   string    `json:"sourceUrl,omitempty"`
	Image       *imageDTO `json:"image,omitempty"`
}

// imageDTO carries the image URL plus the attribution the UI must display.
type imageDTO struct {
	URL     string `json:"url"`
	Author  string `json:"author,omitempty"`
	License string `json:"license,omitempty"`
	Source  string `json:"source,omitempty"`
}

type traitDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// --- Handlers ---------------------------------------------------------------

// emoji returns the slug -> emoji map the finder cards use.
func (s *Server) emoji(w http.ResponseWriter, _ *http.Request) {
	if s.Emoji == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, s.Emoji)
}

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
		out = append(out, animalDTO{Slug: a.Slug, Name: a.Name, Description: a.Description, Traits: a.Traits, Score: a.Score})
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

	dto := animalDetailDTO{
		Slug:        d.Slug,
		Name:        d.Name,
		NameNL:      d.NameNL,
		AltNames:    d.AltNames,
		Description: d.Description,
		Traits:      d.Traits,
		SourceURL:   d.SourceURL,
	}
	if d.ImagePath != "" {
		dto.Image = &imageDTO{
			URL:     "/api/v1/animals/" + d.Slug + "/image",
			Author:  d.ImageAuthor,
			License: d.ImageLicense,
			Source:  d.ImageSource,
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// animalImage streams <slug>.webp from imageDir, or 404 if absent.
func (s *Server) animalImage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if s.imageDir == "" || !slugRe.MatchString(slug) {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	path := filepath.Join(s.imageDir, slug+".webp")
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
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
		out = append(out, animalDTO{Slug: a.Slug, Name: a.Name, Description: a.Description, Traits: a.Traits, Score: a.Score})
	}
	writeJSON(w, http.StatusOK, out)
}

// similarByTraits ranks animals by Jaccard overlap with the selected trait
// profile (Similarity mode). Reuses the include/exclude query params.
func (s *Server) similarByTraits(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := validLang(q.Get("lang"))
	include := splitCSV(q.Get("include"))
	exclude := splitCSV(q.Get("exclude"))

	animals, err := s.cat.SimilarByTraits(r.Context(), include, exclude, lang, 60)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not compute similar animals")
		return
	}
	out := make([]animalDTO, 0, len(animals))
	for _, a := range animals {
		out = append(out, animalDTO{Slug: a.Slug, Name: a.Name, Description: a.Description, Traits: a.Traits, Score: a.Score})
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
