package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
)

type userContextKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey{}).(User)
	return user, ok
}

func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign-in required")
			return
		}
		user, csrfHash, err := h.repo.GetSession(r.Context(), sha256.Sum256([]byte(cookie.Value)))
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "sign-in required")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "could not load session")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if !h.validCSRF(r, csrfHash) {
				writeError(w, http.StatusForbidden, "invalid_csrf", "CSRF token required")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func (h *Handler) validCSRF(r *http.Request, stored [32]byte) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && origin != h.appOrigin {
		return false
	}
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
		return false
	}
	actual := sha256.Sum256([]byte(header))
	return subtle.ConstantTimeCompare(actual[:], stored[:]) == 1
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		User User `json:"user"`
	}{User: user})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(sessionCookie)
	if err := h.repo.RevokeSession(r.Context(), sha256.Sum256([]byte(cookie.Value))); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not revoke session")
		return
	}
	h.setCookie(w, sessionCookie, "", -1, true, "/")
	h.setCookie(w, csrfCookie, "", -1, false, "/")
	w.WriteHeader(http.StatusNoContent)
}
