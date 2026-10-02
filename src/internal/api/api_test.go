package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Niklasvdm/TotemAppIT/internal/store"
)

// fakeCatalog implements Catalog without a database, so the HTTP layer can be
// tested in isolation (this is why the interface lives in the consumer).
type fakeCatalog struct {
	gotFilter store.Filter
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
