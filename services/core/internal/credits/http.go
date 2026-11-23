// Package credits serves the signed-in user's credit balance and history.
package credits

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

const historyLimit = 50

// Handler serves the credits routes.
type Handler struct{ q *store.Queries }

// NewHandler returns a Handler reading from db.
func NewHandler(db store.DBTX) *Handler { return &Handler{q: store.New(db)} }

// Routes registers the credits routes inside the session group.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/me/credits", h.balance)
	r.Get("/me/credits/history", h.history)
}

func (h *Handler) balance(w http.ResponseWriter, r *http.Request) {
	n, err := h.q.GetCreditBalance(r.Context(), auth.PrincipalFrom(r).User.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"balance": n})
}

type entryBody struct {
	ID        string    `json:"id"`
	Delta     int64     `json:"delta"`
	Kind      string    `json:"kind"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListCreditEntries(r.Context(), store.ListCreditEntriesParams{UserID: auth.PrincipalFrom(r).User.ID, RowLimit: historyLimit})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
		return
	}
	out := make([]entryBody, 0, len(rows))
	for _, e := range rows {
		out = append(out, entryBody{ID: e.ID.String(), Delta: e.Delta, Kind: e.Kind, Note: e.Note, CreatedAt: e.CreatedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]entryBody{"entries": out})
}
