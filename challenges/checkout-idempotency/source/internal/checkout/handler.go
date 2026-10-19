package checkout

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /checkout", h.checkout)
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer r.Body.Close()
	var request Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		writeError(w, http.StatusBadRequest, "expected one JSON value")
		return
	}
	order, err := h.service.Checkout(r.Context(), r.Header.Get("X-User-ID"), r.Header.Get("Idempotency-Key"), request)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRequest):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrDuplicateKey):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "checkout failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(order)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
