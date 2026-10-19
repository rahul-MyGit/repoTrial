package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type catalogRepo struct{ items []Problem }

func (r catalogRepo) List(_ context.Context, after string, limit int) ([]Problem, error) {
	out := []Problem{}
	for _, item := range r.items {
		if item.Slug > after {
			out = append(out, item)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r catalogRepo) Get(_ context.Context, slug string) (Problem, error) {
	for _, item := range r.items {
		if item.Slug == slug {
			return item, nil
		}
	}
	return Problem{}, ErrNotFound
}

func TestCatalogPaginationAndValidation(t *testing.T) {
	items := []Problem{
		{Slug: "alpha", Version: 1, Title: "Alpha", Language: "go", Tags: []string{}},
		{Slug: "beta", Version: 2, Title: "Beta", Language: "go", Tags: []string{}},
		{Slug: "gamma", Version: 1, Title: "Gamma", Language: "go", Tags: []string{}},
	}
	mux := http.NewServeMux()
	NewHandler(catalogRepo{items: items}).Register(mux)
	read := func(path string) (int, struct {
		Items      []Problem `json:"items"`
		NextCursor *string   `json:"nextCursor"`
	}) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var body struct {
			Items      []Problem `json:"items"`
			NextCursor *string   `json:"nextCursor"`
		}
		if w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, body
	}

	status, first := read("/api/v1/problems?limit=2")
	if status != http.StatusOK || len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("first page: status=%d body=%+v", status, first)
	}
	status, second := read("/api/v1/problems?limit=2&cursor=" + url.QueryEscape(*first.NextCursor))
	if status != http.StatusOK || len(second.Items) != 1 || second.Items[0].Slug != "gamma" || second.NextCursor != nil {
		t.Fatalf("second page: status=%d body=%+v", status, second)
	}
	for _, path := range []string{
		"/api/v1/problems?limit=0",
		"/api/v1/problems?limit=101",
		"/api/v1/problems?cursor=invalid!",
	} {
		if status, _ := read(path); status != http.StatusBadRequest {
			t.Fatalf("invalid query %q returned %d", path, status)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/problems/missing", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing problem returned %d", w.Code)
	}
}
