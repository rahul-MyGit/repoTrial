package httpapi

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rahul-MyGit/repoTrial/apps/api/internal/attempts"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/auth"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/catalog"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/diagnosis"
)

type testAuthRepo struct{}

func (testAuthRepo) CreateSession(context.Context, auth.Identity, [32]byte, [32]byte, time.Time) (auth.User, error) {
	return auth.User{}, nil
}
func (testAuthRepo) GetSession(context.Context, [32]byte) (auth.User, [32]byte, error) {
	return auth.User{ID: "00000000-0000-0000-0000-000000000001"}, sha256.Sum256([]byte("csrf")), nil
}
func (testAuthRepo) RevokeSession(context.Context, [32]byte) error { return nil }

type testCatalogRepo struct{}

func (testCatalogRepo) List(context.Context, string, int) ([]catalog.Problem, error) {
	return []catalog.Problem{}, nil
}
func (testCatalogRepo) Get(context.Context, string) (catalog.Problem, error) {
	return catalog.Problem{}, catalog.ErrNotFound
}

type testAttemptRepo struct{}

func (testAttemptRepo) Create(context.Context, string, string, string) (attempts.Attempt, error) {
	return attempts.Attempt{ID: "00000000-0000-0000-0000-000000000002"}, nil
}
func (testAttemptRepo) Get(context.Context, string, string) (attempts.Attempt, error) {
	return attempts.Attempt{}, attempts.ErrNotFound
}

type testDiagnosisRepo struct{}

func (testDiagnosisRepo) Record(context.Context, string, string, diagnosis.Input) (diagnosis.Diagnosis, error) {
	return diagnosis.Diagnosis{}, diagnosis.ErrConflict
}
func (testDiagnosisRepo) List(context.Context, string, string) ([]diagnosis.Diagnosis, error) {
	return []diagnosis.Diagnosis{}, nil
}

func testApplication(t *testing.T) http.Handler {
	t.Helper()
	authHandler, err := auth.NewHandler(testAuthRepo{}, auth.Config{
		ClientID: "test", ClientSecret: "test", StateSecret: "test",
		RedirectURL: "http://localhost:3000/api/v1/auth/google/callback",
		AppURL:      "http://localhost:3000",
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewApplicationHandler(Services{
		Auth:      authHandler,
		Catalog:   catalog.NewHandler(testCatalogRepo{}),
		Attempts:  attempts.NewService(testAttemptRepo{}),
		Diagnoses: diagnosis.NewService(testDiagnosisRepo{}),
	})
}

func TestApplicationPublicCatalogAndProtectedAttempt(t *testing.T) {
	handler := testApplication(t)
	public := httptest.NewRecorder()
	handler.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/problems", nil))
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), `"items":[]`) {
		t.Fatalf("public catalog: status %d, body %q", public.Code, public.Body.String())
	}
	protected := httptest.NewRecorder()
	handler.ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/api/v1/attempts/00000000-0000-0000-0000-000000000002", nil))
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("protected attempt: status %d", protected.Code)
	}
}

func TestApplicationRequiresCSRFOnAttemptCreation(t *testing.T) {
	handler := testApplication(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/attempts", strings.NewReader(`{"problemSlug":"checkout-idempotency"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "new-attempt")
	request.AddCookie(&http.Cookie{Name: "repotrial_session", Value: "session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF: status %d, body %q", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/attempts", strings.NewReader(`{"problemSlug":"checkout-idempotency"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "new-attempt")
	request.Header.Set("X-CSRF-Token", "csrf")
	request.AddCookie(&http.Cookie{Name: "repotrial_session", Value: "session"})
	request.AddCookie(&http.Cookie{Name: "repotrial_csrf", Value: "csrf"})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("mutation with CSRF: status %d, body %q", response.Code, response.Body.String())
	}
}
