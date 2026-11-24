package auth

import (
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Gitax18/readmely/services/core/internal/contract/corepublic"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
	"github.com/Gitax18/readmely/services/core/internal/oauth"
)

// OAuthCookieName binds a sign-in attempt to the browser that started it. Like the session
// cookie it carries the __Host- prefix.
const OAuthCookieName = "__Host-rm_oauth"

func setOAuthCookie(w http.ResponseWriter, binding string) {
	http.SetCookie(w, &http.Cookie{
		Name: OAuthCookieName, Value: binding, Path: "/",
		MaxAge: int(oauth.AttemptTTL / time.Second),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func clearOAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: OAuthCookieName, Value: "", Path: "/", MaxAge: -1,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// redirect answers 302. Every Location this file sends is either the provider's authorization
// URL, built from constants and the configured client id, or one of the fixed paths below.
func redirect(w http.ResponseWriter, to string) {
	w.Header().Set("Location", to)
	w.WriteHeader(http.StatusFound) // no-store and no-referrer come from the router for every response
}

func (h *Handler) providers(w http.ResponseWriter, _ *http.Request) {
	out := corepublic.AuthProviders{Providers: []corepublic.Provider{}}
	for _, name := range h.svc.Providers() {
		out.Providers = append(out.Providers, corepublic.Provider(name))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) oauthStart(w http.ResponseWriter, r *http.Request) {
	authURL, binding, err := h.svc.StartOAuth(r.Context(), chi.URLParam(r, "provider"), h.addr.Addr(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	setOAuthCookie(w, binding)
	redirect(w, authURL)
}

func (h *Handler) oauthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c := Callback{
		Provider: chi.URLParam(r, "provider"), Code: q.Get("code"), State: q.Get("state"),
		ProviderError: q.Get("error"), IP: h.addr.Addr(r), UserAgent: r.UserAgent(),
	}
	if ck, err := r.Cookie(OAuthCookieName); err == nil {
		c.Binding = ck.Value
	}
	if ck, err := r.Cookie(CookieName); err == nil {
		c.SessionToken = ck.Value
	}
	res, err := h.svc.FinishOAuth(r.Context(), c)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	clearOAuthCookie(w) // the attempt is over, whatever the outcome
	if res.LoggedIn != nil {
		setSessionCookie(w, res.LoggedIn.Token)
	}
	if res.ErrCode != "" {
		redirect(w, "/login?error="+url.QueryEscape(res.ErrCode))
		return
	}
	if res.Next != "" {
		redirect(w, res.Next)
		return
	}
	redirect(w, "/account")
}

func (h *Handler) listIdentities(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.ListIdentities(r.Context(), principal(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]corepublic.Identity, 0, len(rows))
	for _, i := range rows {
		out = append(out, corepublic.Identity{Provider: corepublic.Provider(i.Provider), Email: i.Email, CreatedAt: i.CreatedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, corepublic.IdentityList{Identities: out})
}

func (h *Handler) startConnect(w http.ResponseWriter, r *http.Request) {
	authURL, binding, err := h.svc.StartConnect(r.Context(), principal(r), chi.URLParam(r, "provider"), h.addr.Addr(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	setOAuthCookie(w, binding)
	httpx.WriteJSON(w, http.StatusOK, corepublic.AuthorizationUrl{AuthorizationUrl: authURL})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if !ValidProvider(provider) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found")
		return
	}
	if err := h.svc.DisconnectIdentity(r.Context(), principal(r), provider, r.UserAgent()); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) githubAccess(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.GitHubAccess(r.Context(), principal(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, corepublic.GitHubAccess{Enabled: st.Enabled, Connected: st.Connected, Granted: st.Granted})
}

func (h *Handler) startGitHubAccess(w http.ResponseWriter, r *http.Request) {
	authURL, binding, err := h.svc.StartGitHubAccess(r.Context(), principal(r), h.addr.Addr(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	setOAuthCookie(w, binding)
	httpx.WriteJSON(w, http.StatusOK, corepublic.AuthorizationUrl{AuthorizationUrl: authURL})
}

func (h *Handler) removeGitHubAccess(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveGitHubAccess(r.Context(), principal(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
