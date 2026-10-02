// Package api is the HTTP transport: it maps requests to Catalog calls and
// domain values to JSON DTOs. It knows nothing about SQL or encryption.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Niklasvdm/TotemAppIT/internal/game"
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
	AddSuggestion(ctx context.Context, name, note string) error
	AddReport(ctx context.Context, slug, reason, note string) error
}

// Server holds the router and its dependencies.
type Server struct {
	cat    Catalog
	images fs.FS             // <slug>.webp files; nil when images aren't deployed
	emoji  map[string]string // slug -> emoji, served at /api/v1/emoji
	games  *game.Registry    // live multiplayer game rooms
	Router http.Handler

	// gameOrigins are extra host patterns allowed to open a game WebSocket, on
	// top of the always-permitted same-origin case. See WithGameOrigins.
	gameOrigins []string
}

// Option adjusts a Server at construction. Options keep New's signature stable
// as the server grows optional dependencies.
type Option func(*Server)

// WithGameOrigins authorises extra Origin host patterns for the game
// WebSocket (e.g. "127.0.0.1:5173" for the Vite dev proxy, which forwards its
// own Host so the browser's Origin no longer matches). Same-origin requests are
// always allowed, so this only ever widens access — leave it empty in
// production.
func WithGameOrigins(patterns []string) Option {
	return func(s *Server) { s.gameOrigins = patterns }
}

// New wires the routes and middleware. images is the filesystem of animal images
// (nil → the image route 404s); emoji is the slug→emoji map for /api/v1/emoji.
//
// The returned Server owns a game registry with background goroutines; callers
// that outlive a single request should Close it.
func New(cat Catalog, images fs.FS, emoji map[string]string, opts ...Option) *Server {
	s := &Server{cat: cat, images: images, emoji: emoji, games: game.NewRegistry()}
	for _, opt := range opts {
		opt(s)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(secureHeaders)

	r.Get("/healthz", s.health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/animals", s.listAnimals)
		r.Get("/animals/{slug}", s.getAnimal)
		r.Get("/animals/{slug}/similar", s.similar)
		r.Get("/similar", s.similarByTraits)
		r.Get("/animals/{slug}/image", s.animalImage)
		r.Get("/traits", s.listTraits)
		r.Get("/emoji", s.emojiMap)

		// Public write endpoints: rate-limited and strictly validated.
		r.Group(func(r chi.Router) {
			r.Use(newRateLimiter(10, time.Minute).middleware) // 10 writes/min/IP
			r.Post("/suggestions", s.createSuggestion)
			r.Post("/animals/{slug}/reports", s.createReport)
		})

		// Multiplayer games. Creating a room allocates a goroutine and a tick
		// loop, so it is rate-limited; the socket itself is not, since a long
		// -lived connection is the point (it has its own per-frame budget).
		r.Route("/games", func(r chi.Router) {
			r.With(newRateLimiter(20, time.Minute).middleware).Post("/rooms", s.createRoom)
			r.Get("/ws", s.gameWS)
		})
	})

	s.Router = r
	return s
}

// Close releases the server's background resources (the game registry's reaper
// and every open room).
func (s *Server) Close() {
	s.games.Close()
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
	NameIT      string    `json:"nameIt,omitempty"`
	NameEN      string    `json:"nameEn,omitempty"`
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

// emojiMap returns the slug -> emoji map the finder cards use.
func (s *Server) emojiMap(w http.ResponseWriter, _ *http.Request) {
	if s.emoji == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, s.emoji)
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
		NameIT:      d.NameIT,
		NameEN:      d.NameEN,
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

// animalImage streams <slug>.webp from the images FS, or 404 if absent.
func (s *Server) animalImage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if s.images == nil || !slugRe.MatchString(slug) {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	name := slug + ".webp"
	if _, err := fs.Stat(s.images, name); err != nil {
		writeErr(w, http.StatusNotFound, "no image")
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFileFS(w, r, s.images, name)
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
	limit := 60
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}

	animals, err := s.cat.SimilarByTraits(r.Context(), include, exclude, lang, limit)
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

// --- write handlers (community feedback) ------------------------------------

type suggestionReq struct {
	Name string `json:"name"`
	Note string `json:"note"`
}

type reportReq struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// createSuggestion accepts a new-animal suggestion. Deduped + counted in the
// store; always answers "received" so repeat/duplicate state isn't enumerable.
func (s *Server) createSuggestion(w http.ResponseWriter, r *http.Request) {
	var req suggestionReq
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ok := cleanName(req.Name)
	if !ok {
		writeErr(w, http.StatusBadRequest, "name must be 1–60 letters (spaces, - ' . ( ) allowed)")
		return
	}
	note, ok := cleanNote(req.Note)
	if !ok {
		writeErr(w, http.StatusBadRequest, "note too long or contains disallowed characters")
		return
	}
	if err := s.cat.AddSuggestion(r.Context(), name, note); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save suggestion")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}

// createReport accepts a report against an existing animal.
func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !slugRe.MatchString(slug) {
		writeErr(w, http.StatusNotFound, "animal not found")
		return
	}
	var req reportReq
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if !store.ValidReportReason(req.Reason) {
		writeErr(w, http.StatusBadRequest, "invalid reason")
		return
	}
	note, ok := cleanNote(req.Note)
	if !ok {
		writeErr(w, http.StatusBadRequest, "note too long or contains disallowed characters")
		return
	}
	err := s.cat.AddReport(r.Context(), slug, req.Reason, note)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "animal not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save report")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "received"})
}

// --- middleware -------------------------------------------------------------

// rateLimiter is a tiny fixed-window per-IP limiter for the write endpoints,
// so a single client can't flood the suggestion/report tables. In-memory and
// best-effort (resets on restart); the WAF in front handles broader abuse.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
}

type window struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, w time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string]*window), limit: limit, window: w}
}

func (rl *rateLimiter) allow(ip string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	w, ok := rl.hits[ip]
	if !ok || now.After(w.reset) {
		rl.hits[ip] = &window{count: 1, reset: now.Add(rl.window)}
		// Opportunistic cleanup so the map can't grow unbounded.
		if len(rl.hits) > 10000 {
			for k, v := range rl.hits {
				if now.After(v.reset) {
					delete(rl.hits, k)
				}
			}
		}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RealIP middleware has already normalized r.RemoteAddr.
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		if !rl.allow(ip, time.Now()) {
			writeErr(w, http.StatusTooManyRequests, "slow down — too many submissions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders sets conservative response headers. The API is JSON + images
// and (later) the embedded SPA, so framing is denied and MIME sniffing is off.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// Conservative CSP for the API (JSON + images, fetched by the SPA). It
		// blocks framing and <base> hijacking without constraining resource loads;
		// a document-scoped policy (script-src 'self', …) comes with the SPA embed.
		h.Set("Content-Security-Policy", "frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
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

// decodeJSON reads a small JSON body (max 4 KiB) into dst, rejecting unknown
// fields and trailing garbage. Bounds the body so a write endpoint can't be
// used to exhaust memory.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	if dec.More() {
		return errors.New("unexpected trailing data")
	}
	return nil
}

// cleanName validates a submitted animal name: 1–60 runes of letters/marks plus
// a small punctuation set. Returns the trimmed name and whether it's valid.
// Rejects digits, angle brackets and control characters outright.
func cleanName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 1 || n > 60 {
		return "", false
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), r == ' ':
		case r == '-' || r == '\'' || r == '’' || r == '.' || r == '(' || r == ')':
		default:
			return "", false
		}
	}
	return s, true
}

// cleanNote validates an optional free-text note: up to 280 runes, no control
// characters and no angle brackets (defence in depth against stored markup).
func cleanNote(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 280 {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			return "", false
		}
	}
	return s, true
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
