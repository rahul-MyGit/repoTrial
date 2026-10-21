package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rahul-MyGit/repoTrial/apps/api/internal/attempts"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/auth"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/catalog"
	"github.com/rahul-MyGit/repoTrial/apps/api/internal/diagnosis"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Services struct {
	Auth      *auth.Handler
	Catalog   *catalog.Handler
	Attempts  *attempts.Service
	Files     *attempts.FileService
	Diagnoses *diagnosis.Service
}

func NewApplicationHandler(s Services) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, Health{Status: "ok"})
	})
	s.Auth.Register(mux)
	s.Catalog.Register(mux)
	protected := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, s.Auth.Authenticate(handler))
	}
	protected("POST /api/v1/attempts", s.createAttempt)
	protected("GET /api/v1/attempts/{attemptId}", s.getAttempt)
	protected("POST /api/v1/attempts/{attemptId}/diagnoses", s.recordDiagnosis)
	protected("GET /api/v1/attempts/{attemptId}/diagnoses", s.listDiagnoses)
	protected("GET /api/v1/attempts/{attemptId}/files", s.listFiles)
	protected("GET /api/v1/attempts/{attemptId}/file", s.readFile)
	return mux
}

func (s Services) createAttempt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProblemSlug string `json:"problemSlug"`
	}
	if err := decode(r, &body); err != nil || body.ProblemSlug == "" {
		fail(w, http.StatusBadRequest, "invalid_request", "problemSlug is required")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		fail(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must contain 1 to 128 characters")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Attempts.Create(r.Context(), user.ID, body.ProblemSlug, key)
	if errors.Is(err, attempts.ErrNotFound) {
		fail(w, http.StatusNotFound, "not_found", "problem not found")
		return
	}
	if errors.Is(err, attempts.ErrConflict) {
		fail(w, http.StatusConflict, "conflict", "Idempotency-Key belongs to another request")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "internal_error", "could not create attempt")
		return
	}
	respond(w, http.StatusCreated, result)
}

func (s Services) getAttempt(w http.ResponseWriter, r *http.Request) {
	id, ok := attemptID(w, r)
	if !ok {
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Attempts.Get(r.Context(), user.ID, id)
	if handleAttemptError(w, err) {
		return
	}
	respond(w, http.StatusOK, result)
}

func (s Services) recordDiagnosis(w http.ResponseWriter, r *http.Request) {
	id, ok := attemptID(w, r)
	if !ok {
		return
	}
	var body diagnosis.Input
	if err := decode(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_request", "invalid diagnosis body")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Diagnoses.Record(r.Context(), user.ID, id, body)
	if handleDiagnosisError(w, err) {
		return
	}
	respond(w, http.StatusCreated, result)
}

func (s Services) listDiagnoses(w http.ResponseWriter, r *http.Request) {
	id, ok := attemptID(w, r)
	if !ok {
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Diagnoses.List(r.Context(), user.ID, id)
	if handleDiagnosisError(w, err) {
		return
	}
	respond(w, http.StatusOK, result)
}

func (s Services) listFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := attemptID(w, r)
	if !ok {
		return
	}
	revision, ok := requestedRevision(w, r)
	if !ok {
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Files.List(r.Context(), user.ID, id, revision)
	if handleAttemptError(w, err) {
		return
	}
	respond(w, http.StatusOK, result)
}

func (s Services) readFile(w http.ResponseWriter, r *http.Request) {
	id, ok := attemptID(w, r)
	if !ok {
		return
	}
	revision, ok := requestedRevision(w, r)
	if !ok {
		return
	}
	name := r.URL.Query().Get("path")
	if name == "" {
		fail(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	result, err := s.Files.Read(r.Context(), user.ID, id, name, revision)
	if handleAttemptError(w, err) {
		return
	}
	respond(w, http.StatusOK, result)
}

func attemptID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("attemptId")
	if !uuidPattern.MatchString(id) {
		fail(w, http.StatusNotFound, "not_found", "attempt not found")
		return "", false
	}
	return id, true
}

func requestedRevision(w http.ResponseWriter, r *http.Request) (*int, bool) {
	raw := r.URL.Query().Get("revision")
	if raw == "" {
		return nil, true
	}
	revision, err := strconv.Atoi(raw)
	if err != nil || revision < 0 {
		fail(w, http.StatusBadRequest, "invalid_revision", "revision must be a non-negative integer")
		return nil, false
	}
	return &revision, true
}

func handleAttemptError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, attempts.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found", "attempt or file not found")
	case errors.Is(err, attempts.ErrLocked), errors.Is(err, attempts.ErrConflict):
		fail(w, http.StatusConflict, "attempt_locked", "record a diagnosis after the baseline run to inspect files")
	default:
		fail(w, http.StatusInternalServerError, "internal_error", "could not load attempt")
	}
	return true
}

func handleDiagnosisError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, diagnosis.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found", "attempt not found")
	case errors.Is(err, diagnosis.ErrInvalid):
		fail(w, http.StatusBadRequest, "invalid_diagnosis", "diagnosis fields are required and must fit their limits")
	case errors.Is(err, diagnosis.ErrConflict):
		fail(w, http.StatusConflict, "invalid_state", "a failed baseline run is required before the first diagnosis")
	default:
		fail(w, http.StatusInternalServerError, "internal_error", "could not record diagnosis")
	}
	return true
}

func decode(r *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
	if err != nil || len(body) > 16<<10 {
		return errors.New("request body exceeds 16 KiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
