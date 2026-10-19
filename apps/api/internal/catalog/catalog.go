package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("problem not found")

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

type Problem struct {
	Slug       string   `json:"slug"`
	Version    int      `json:"version"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Language   string   `json:"language"`
	Difficulty string   `json:"difficulty"`
	Tags       []string `json:"tags"`
}

type Repository interface {
	List(context.Context, string, int) ([]Problem, error)
	Get(context.Context, string) (Problem, error)
}

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const selectPublished = `SELECT p.slug, v.version, p.title, p.summary, p.language, p.difficulty, p.tags
FROM problems p
JOIN LATERAL (
  SELECT version FROM challenge_versions
  WHERE problem_id = p.id AND published_at IS NOT NULL
  ORDER BY version DESC LIMIT 1
) v ON true
WHERE p.is_published`

func (r *PostgresRepository) List(ctx context.Context, after string, limit int) ([]Problem, error) {
	rows, err := r.pool.Query(ctx, selectPublished+` AND p.slug > $1 ORDER BY p.slug LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Problem{}
	for rows.Next() {
		var item Problem
		if err := rows.Scan(
			&item.Slug,
			&item.Version,
			&item.Title,
			&item.Summary,
			&item.Language,
			&item.Difficulty,
			&item.Tags,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, slug string) (Problem, error) {
	var item Problem
	err := r.pool.QueryRow(ctx, selectPublished+` AND p.slug = $1`, slug).Scan(
		&item.Slug,
		&item.Version,
		&item.Title,
		&item.Summary,
		&item.Language,
		&item.Difficulty,
		&item.Tags,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Problem{}, ErrNotFound
	}
	return item, err
}

type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/problems", h.list)
	mux.HandleFunc("GET /api/v1/problems/{slug}", h.get)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	after := ""
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || !slugPattern.Match(decoded) {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "invalid catalog cursor")
			return
		}
		after = string(decoded)
	}

	items, err := h.repo.List(r.Context(), after, limit+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list problems")
		return
	}
	var nextCursor *string
	if len(items) > limit {
		items = items[:limit]
		cursor := base64.RawURLEncoding.EncodeToString([]byte(items[len(items)-1].Slug))
		nextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, struct {
		Items      []Problem `json:"items"`
		NextCursor *string   `json:"nextCursor"`
	}{Items: items, NextCursor: nextCursor})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if !slugPattern.MatchString(slug) {
		writeError(w, http.StatusNotFound, "not_found", "problem not found")
		return
	}
	item, err := h.repo.Get(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "problem not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load problem")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
