package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/contract/corepublic"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
)

// CookieName carries the __Host- prefix: browsers then refuse the cookie unless it is Secure,
// has Path=/ and no Domain, so a sibling subdomain cannot plant or overwrite it.
const CookieName = "__Host-rm_session"

const (
	csrfHeader    = "X-CSRF-Token"
	maxAuthBody   = 8 << 10
	contentTypeJS = "application/json"
)

// Handler serves the /v1 account endpoints from the public listener.
type Handler struct {
	svc       *Service
	addr      *clientip.Resolver
	webOrigin string
	logger    *slog.Logger
	extras    []func(chi.Router)
}

// NewHandler builds the HTTP layer. webOrigin is the one origin allowed to make
// state-changing requests (WEB_ORIGIN).
func NewHandler(svc *Service, addr *clientip.Resolver, webOrigin string, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, addr: addr, webOrigin: webOrigin, logger: logger}
}

// AddRoutes lets another feature register routes that need a signed-in user. They are mounted under
// /v1 inside the session group, so the Origin, content type, session and CSRF checks apply to them
// exactly as to the account routes. Call it before Mount.
func (h *Handler) AddRoutes(f func(chi.Router)) { h.extras = append(h.extras, f) }

// Mount registers the routes.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(h.guard)
		r.Post("/auth/email/start", h.start)
		r.Post("/auth/email/verify", h.verify)
		r.Get("/auth/providers", h.providers)
		r.Get("/auth/{provider}/start", h.oauthStart)
		r.Get("/auth/{provider}/callback", h.oauthCallback)
		r.Group(func(r chi.Router) {
			r.Use(h.session)
			r.Post("/auth/logout", h.logout)
			r.Get("/me", h.me)
			r.Patch("/me", h.updateMe)
			r.Get("/me/sessions", h.listSessions)
			r.Delete("/me/sessions", h.revokeOthers)
			r.Delete("/me/sessions/{id}", h.revoke)
			r.Post("/me/deletion/start", h.startDeletion)
			r.Post("/me/deletion/confirm", h.confirmDeletion)
			r.Get("/me/identities", h.listIdentities)
			r.Post("/me/identities/{provider}/start", h.startConnect)
			r.Delete("/me/identities/{provider}", h.disconnect)
			for _, extra := range h.extras {
				extra(r)
			}
		})
	})
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// guard is the cross-site defence for every state-changing request: the Origin must be ours
// (a missing one is refused too, since browsers always send it on these methods), and a body
// must be JSON, which a plain HTML form cannot produce.
func (h *Handler) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unsafeMethod(r.Method) {
			if r.Header.Get("Origin") != h.webOrigin {
				httpx.WriteError(w, r, http.StatusForbidden, "forbidden")
				return
			}
			if r.ContentLength != 0 {
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mt != contentTypeJS {
					httpx.WriteError(w, r, http.StatusForbidden, "forbidden")
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxAuthBody)
		}
		next.ServeHTTP(w, r)
	})
}

type principalKey struct{}

// session requires a live session and, for state-changing requests, the session's CSRF token.
func (h *Handler) session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated")
			return
		}
		p, err := h.svc.Authenticate(r.Context(), c.Value)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if unsafeMethod(r.Method) && !h.svc.Keys().CSRFMatches(p.Session.ID, r.Header.Get(csrfHeader)) {
			httpx.WriteError(w, r, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func principal(r *http.Request) *Principal {
	p, _ := r.Context().Value(principalKey{}).(*Principal)
	return p
}

// PrincipalFrom is who a request made inside the session group is acting as.
func PrincipalFrom(r *http.Request) *Principal { return principal(r) }

// fail turns a service error into the documented response. Anything unexpected is logged
// (without request content) and answered with a generic 500.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var rl *RateLimitedError
	switch {
	case errors.As(err, &rl):
		secs := max(int(rl.RetryAfter/time.Second), 1)
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, ErrInvalidCode):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_code")
	case errors.Is(err, ErrInvalidEmail), errors.Is(err, ErrInvalidName):
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, ErrUnauthenticated):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthenticated")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "conflict")
	default:
		h.logger.ErrorContext(r.Context(), "request failed",
			"request_id", httpx.RequestIDFrom(r.Context()), "path", r.URL.Path, "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error")
	}
}

// decode reads exactly one JSON object with no unknown fields.
func decode(r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	_, err := dec.Token()
	return errors.Is(err, io.EOF)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	var req corepublic.StartEmailLoginRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.svc.StartLogin(r.Context(), req.Email, h.addr.Addr(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, corepublic.Accepted{Status: corepublic.AcceptedStatusAccepted})
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var req corepublic.VerifyEmailLoginRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	in, err := h.svc.VerifyLogin(r.Context(), req.Email, req.Code, h.addr.Addr(r), r.UserAgent())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	setSessionCookie(w, in.Token)
	httpx.WriteJSON(w, http.StatusOK, h.me200(in.User, in.Session.ID))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context(), principal(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	httpx.WriteJSON(w, http.StatusOK, h.me200(p.User, p.Session.ID))
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	var req corepublic.UpdateMeRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	u, err := h.svc.UpdateDisplayName(r.Context(), principal(r), req.DisplayName)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userBody(u))
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	rows, err := h.svc.ListSessions(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]corepublic.Session, 0, len(rows))
	for _, s := range rows {
		out = append(out, corepublic.Session{
			Id:         s.ID,
			CreatedAt:  s.CreatedAt,
			LastSeenAt: s.LastSeenAt,
			Device:     DeviceLabel(s.UserAgent),
			Current:    s.ID == p.Session.ID,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, corepublic.SessionList{Sessions: out})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	p := principal(r)
	if err := h.svc.RevokeSession(r.Context(), p, id); err != nil {
		h.fail(w, r, err)
		return
	}
	if id == p.Session.ID {
		clearSessionCookie(w) // revoking the session you are on is signing out
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revokeOthers(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeOtherSessions(r.Context(), principal(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) startDeletion(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.StartDeletion(r.Context(), principal(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, corepublic.Accepted{Status: corepublic.AcceptedStatusAccepted})
}

func (h *Handler) confirmDeletion(w http.ResponseWriter, r *http.Request) {
	var req corepublic.ConfirmDeletionRequest
	if !decode(r, &req) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.svc.ConfirmDeletion(r.Context(), principal(r), req.Code); err != nil {
		h.fail(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func userBody(u store.User) corepublic.User {
	return corepublic.User{Id: u.ID, Email: u.Email, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt}
}

func (h *Handler) me200(u store.User, sessionID uuid.UUID) corepublic.Me {
	return corepublic.Me{User: userBody(u), CsrfToken: h.svc.Keys().CSRFToken(sessionID)}
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/",
		MaxAge: int(SessionAbsolute / time.Second),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}
