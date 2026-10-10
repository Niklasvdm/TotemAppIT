package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

// fakeCatalog implements Catalog without a database, so the HTTP layer can be
// tested in isolation (this is why the interface lives in the consumer).
type fakeCatalog struct {
	gotFilter store.Filter
	gotSugg   [2]string // name, note
	gotReport [3]string // slug, reason, note
	gotGameRep [3]string // game, reason, note
}

func (f *fakeCatalog) ListAnimals(_ context.Context, flt store.Filter) ([]store.Animal, error) {
	f.gotFilter = flt
	return []store.Animal{{Slug: "vos", Name: "Volpe", Traits: []string{"astuto"}}}, nil
}
func (f *fakeCatalog) GetAnimal(_ context.Context, slug, _ string) (*store.AnimalDetail, error) {
	if slug != "vos" {
		return nil, store.ErrNotFound
	}
	return &store.AnimalDetail{Slug: "vos", Name: "Volpe", NameNL: "vos"}, nil
}
func (f *fakeCatalog) Similar(context.Context, string, string, int) ([]store.Animal, error) {
	return []store.Animal{{Slug: "wolf", Name: "Lupo"}}, nil
}
func (f *fakeCatalog) SimilarByTraits(context.Context, []string, []string, string, int) ([]store.Animal, error) {
	return []store.Animal{{Slug: "wolf", Name: "Lupo"}}, nil
}
func (f *fakeCatalog) ListTraits(context.Context, string) ([]store.Trait, error) {
	return []store.Trait{{Key: "sluw", Label: "astuto", Count: 2}}, nil
}
func (f *fakeCatalog) AddSuggestion(_ context.Context, name, note string) error {
	f.gotSugg = [2]string{name, note}
	return nil
}
func (f *fakeCatalog) AddReport(_ context.Context, slug, reason, note string) error {
	if slug != "vos" {
		return store.ErrNotFound
	}
	f.gotReport = [3]string{slug, reason, note}
	return nil
}
func (f *fakeCatalog) AddGameReport(_ context.Context, gameSlug, reason, note string) error {
	f.gotGameRep = [3]string{gameSlug, reason, note}
	return nil
}

func do(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestListAnimalsParsesFilter(t *testing.T) {
	fc := &fakeCatalog{}
	srv := New(fc, nil, nil)

	rec := do(t, srv, "/api/v1/animals?lang=en&q=fox&include=sluw,snel&exclude=nat")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if fc.gotFilter.Lang != "en" || fc.gotFilter.Query != "fox" {
		t.Fatalf("filter lang/q: %+v", fc.gotFilter)
	}
	if len(fc.gotFilter.Include) != 2 || len(fc.gotFilter.Exclude) != 1 {
		t.Fatalf("include/exclude parse: %+v", fc.gotFilter)
	}

	var got []animalDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "vos" {
		t.Fatalf("body: %v", got)
	}
}

func TestLangDefaultsToIT(t *testing.T) {
	fc := &fakeCatalog{}
	srv := New(fc, nil, nil)
	_ = do(t, srv, "/api/v1/animals") // no lang param
	if fc.gotFilter.Lang != "it" {
		t.Fatalf("default lang: %q", fc.gotFilter.Lang)
	}
}

func TestGetAnimalNotFound(t *testing.T) {
	srv := New(&fakeCatalog{}, nil, nil)
	if rec := do(t, srv, "/api/v1/animals/nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if rec := do(t, srv, "/api/v1/animals/vos"); rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestAnimalImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vos.webp"), []byte("RIFFfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(&fakeCatalog{}, os.DirFS(dir), nil)

	if rec := do(t, srv, "/api/v1/animals/vos/image"); rec.Code != http.StatusOK {
		t.Fatalf("existing image: want 200, got %d", rec.Code)
	}
	if rec := do(t, srv, "/api/v1/animals/missing/image"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing image: want 404, got %d", rec.Code)
	}
	// path-traversal attempt must not escape the image dir
	if rec := do(t, srv, "/api/v1/animals/..%2f..%2fetc%2fpasswd/image"); rec.Code != http.StatusNotFound {
		t.Fatalf("traversal: want 404, got %d", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	srv := New(&fakeCatalog{}, nil, nil)
	if rec := do(t, srv, "/healthz"); rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("health: %d %q", rec.Code, rec.Body.String())
	}
}

func TestSPAFallback(t *testing.T) {
	spa := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>totem</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	srv := New(&fakeCatalog{}, nil, nil, WithSPA(spa))

	if rec := do(t, srv, "/assets/app.js"); rec.Code != http.StatusOK {
		t.Fatalf("real asset: want 200, got %d", rec.Code)
	}
	for _, p := range []string{"/", "/animal/wolf"} { // root + deep client route
		rec := do(t, srv, p)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "totem") {
			t.Fatalf("SPA %q: want index.html, got %d %q", p, rec.Code, rec.Body.String())
		}
	}
	// The catch-all must NOT shadow the API.
	if rec := do(t, srv, "/api/v1/animals?lang=en"); rec.Code != http.StatusOK {
		t.Fatalf("api shadowed by SPA: %d", rec.Code)
	}
	// No SPA configured → non-API route 404s.
	if rec := do(t, New(&fakeCatalog{}, nil, nil), "/"); rec.Code != http.StatusNotFound {
		t.Fatalf("api-only mode: want 404 at /, got %d", rec.Code)
	}
}

func doPost(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	srv.Router.ServeHTTP(rec, req)
	return rec
}

func TestCreateSuggestion(t *testing.T) {
	fc := &fakeCatalog{}
	srv := New(fc, nil, nil)

	if rec := doPost(t, srv, "/api/v1/suggestions", `{"name":"Red Panda","note":"cute"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("valid suggestion: want 202, got %d (%s)", rec.Code, rec.Body)
	}
	if fc.gotSugg[0] != "Red Panda" || fc.gotSugg[1] != "cute" {
		t.Fatalf("passed through: %+v", fc.gotSugg)
	}

	bad := []string{
		`{"name":""}`,                      // empty
		`{"name":"<script>"}`,              // angle brackets / punctuation
		`{"name":"Fox123"}`,                // digits
		`{"name":"Fox","bogus":1}`,         // unknown field
		`{"name":"Fox","note":"a<b"}`,      // note with angle bracket
	}
	for _, b := range bad {
		if rec := doPost(t, srv, "/api/v1/suggestions", b); rec.Code != http.StatusBadRequest {
			t.Fatalf("want 400 for %s, got %d", b, rec.Code)
		}
	}
}

func TestCreateReport(t *testing.T) {
	fc := &fakeCatalog{}
	srv := New(fc, nil, nil)

	if rec := doPost(t, srv, "/api/v1/animals/vos/reports", `{"reason":"unknown","note":"never heard of it"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("valid report: want 202, got %d (%s)", rec.Code, rec.Body)
	}
	if fc.gotReport[0] != "vos" || fc.gotReport[1] != "unknown" {
		t.Fatalf("passed through: %+v", fc.gotReport)
	}
	if rec := doPost(t, srv, "/api/v1/animals/vos/reports", `{"reason":"nonsense"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad reason: want 400, got %d", rec.Code)
	}
	if rec := doPost(t, srv, "/api/v1/animals/nope/reports", `{"reason":"unknown"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown animal: want 404, got %d", rec.Code)
	}
}

func TestCreateGameReport(t *testing.T) {
	fc := &fakeCatalog{}
	srv := New(fc, nil, nil)

	if rec := doPost(t, srv, "/api/v1/games/reports", `{"game":"codenames","reason":"bug","note":"board froze"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("valid game report: want 202, got %d (%s)", rec.Code, rec.Body)
	}
	if fc.gotGameRep[0] != "codenames" || fc.gotGameRep[1] != "bug" {
		t.Fatalf("passed through: %+v", fc.gotGameRep)
	}
	if rec := doPost(t, srv, "/api/v1/games/reports", `{"game":"codenames","reason":"nonsense"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad reason: want 400, got %d", rec.Code)
	}
	if rec := doPost(t, srv, "/api/v1/games/reports", `{"game":"not-a-game","reason":"bug"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown game: want 400, got %d", rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	srv := New(&fakeCatalog{}, nil, nil)
	got429 := false
	for i := 0; i < 15; i++ {
		rec := doPost(t, srv, "/api/v1/suggestions", `{"name":"Capybara"}`)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("expected a 429 within 15 rapid writes")
	}
}
