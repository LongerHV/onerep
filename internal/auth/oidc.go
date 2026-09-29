package auth

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Each login in progress has its own flow cookie, named flowCookie + its
// state, so logins in several tabs at once don't overwrite each other's.
const (
	flowCookie    = "onerep_oidc_"
	flowCookieTTL = 10 * time.Minute
	// maxLoginFlows bounds the flow cookies a browser holds: starting another
	// login drops the oldest ones, so abandoned logins can't pile up.
	maxLoginFlows = 5
)

// OIDC implements the authorization code flow with PKCE.
type OIDC struct {
	sessions *Sessions
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider // for UserInfo
}

// LoginError is a failed login, as shown to the user.
type LoginError struct {
	Status  int
	Message string
	Next    string // the page to return to after signing in again
}

// LoginFailure renders the page for a failed login.
type LoginFailure func(w http.ResponseWriter, r *http.Request, e LoginError)

// NewOIDC discovers the provider at issuer. baseURL is the public URL of onerep.
func NewOIDC(ctx context.Context, issuer, clientID, clientSecret, baseURL string, sessions *Sessions) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	return &OIDC{
		sessions: sessions,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimSuffix(baseURL, "/") + "/auth/callback",
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"r"`
	Started  int64  `json:"t"` // Unix nanoseconds, to drop the oldest flows first
}

// safeNext only allows local absolute paths as post-login redirect targets.
// Browsers strip tabs and newlines and treat backslashes as slashes, so
// "/\t/evil.example" would become "//evil.example"; such characters are refused.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	for _, c := range next {
		if c < 0x20 || c == 0x7f || c == '\\' {
			return "/"
		}
	}
	if u, err := url.Parse(next); err != nil || u.Scheme != "" || u.Host != "" {
		return "/"
	}
	return next
}

// Login redirects to the identity provider.
func (o *OIDC) Login(w http.ResponseWriter, r *http.Request) {
	state, err1 := randomToken()
	nonce, err2 := randomToken()
	if err := errors.Join(err1, err2); err != nil {
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	fs := flowState{State: state, Nonce: nonce, Verifier: oauth2.GenerateVerifier(),
		Next: safeNext(r.URL.Query().Get("next")), Started: time.Now().UnixNano()}
	o.pruneFlows(w, r)
	raw, _ := json.Marshal(fs)
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookie + state,
		Value:    base64.RawURLEncoding.EncodeToString(raw),
		Path:     "/auth/",
		MaxAge:   int(flowCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   o.sessions.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(fs.Verifier)), http.StatusFound)
}

// pruneFlows drops the oldest flow cookies so that, with the one being
// started, at most maxLoginFlows remain.
func (o *OIDC) pruneFlows(w http.ResponseWriter, r *http.Request) {
	type flow struct {
		name    string
		started int64
	}
	var flows []flow
	for _, c := range r.Cookies() {
		if !strings.HasPrefix(c.Name, flowCookie) {
			continue
		}
		fs, err := decodeFlow(c.Value)
		if err != nil {
			o.clearFlow(w, c.Name)
			continue
		}
		flows = append(flows, flow{c.Name, fs.Started})
	}
	slices.SortFunc(flows, func(a, b flow) int { return cmp.Compare(b.started, a.started) }) // newest first
	for i, f := range flows {
		if i >= maxLoginFlows-1 {
			o.clearFlow(w, f.name)
		}
	}
}

func (o *OIDC) clearFlow(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/auth/", MaxAge: -1,
		HttpOnly: true, Secure: o.sessions.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func decodeFlow(value string) (flowState, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return flowState{}, err
	}
	var fs flowState
	return fs, json.Unmarshal(raw, &fs)
}

// takeFlow reads and clears the flow cookie of the login with the given state.
func (o *OIDC) takeFlow(w http.ResponseWriter, r *http.Request, state string) (flowState, error) {
	// A state is a base64url token (randomToken); anything else can't name a cookie.
	const tokenChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	if state == "" || len(state) > 64 || strings.Trim(state, tokenChars) != "" {
		return flowState{}, errors.New("malformed state")
	}
	c, err := r.Cookie(flowCookie + state)
	if err != nil {
		return flowState{}, errors.New("no login state cookie for this state")
	}
	o.clearFlow(w, c.Name)
	fs, err := decodeFlow(c.Value)
	if err != nil {
		return flowState{}, err
	}
	if fs.State != state {
		return flowState{}, errors.New("state mismatch")
	}
	return fs, nil
}

// Callback returns the handler that completes the login, creating the user on
// first login. fail renders the page for a failed login; nil sends plain text.
func (o *OIDC) Callback(fail LoginFailure) http.HandlerFunc {
	if fail == nil {
		fail = func(w http.ResponseWriter, _ *http.Request, e LoginError) {
			http.Error(w, e.Message, e.Status)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		next, ok := o.callback(w, r, fail)
		if ok {
			http.Redirect(w, r, next, http.StatusSeeOther)
		}
	}
}

const (
	msgExpired  = "This sign-in has expired or was started in another browser. Please try again."
	msgRejected = "Sign-in failed: the identity provider's answer could not be verified."
)

func (o *OIDC) callback(w http.ResponseWriter, r *http.Request, fail LoginFailure) (string, bool) {
	ctx := r.Context()
	q := r.URL.Query()
	fs, flowErr := o.takeFlow(w, r, q.Get("state"))
	next := cmp.Or(fs.Next, "/")
	reject := func(status int, message, reason string, err error) (string, bool) {
		slog.WarnContext(ctx, "oidc callback failed", "reason", reason, "err", err)
		fail(w, r, LoginError{Status: status, Message: message, Next: next})
		return "", false
	}

	if e := q.Get("error"); e != "" {
		if e == "access_denied" {
			return reject(http.StatusForbidden, "Sign-in was cancelled or refused at the identity provider.", "identity provider returned "+e, nil)
		}
		return reject(http.StatusBadRequest, "The identity provider reported an error: "+e+".", "identity provider returned "+e, nil)
	}
	if flowErr != nil {
		return reject(http.StatusBadRequest, msgExpired, "invalid login state", flowErr)
	}

	token, err := o.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(fs.Verifier))
	if err != nil {
		return reject(http.StatusBadRequest, msgRejected, "code exchange", err)
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok {
		return reject(http.StatusBadRequest, msgRejected, "no id_token in token response", nil)
	}
	idToken, err := o.verifier.Verify(ctx, rawID)
	if err != nil {
		return reject(http.StatusBadRequest, msgRejected, "invalid id_token", err)
	}
	if idToken.Nonce != fs.Nonce {
		return reject(http.StatusBadRequest, msgRejected, "nonce mismatch", nil)
	}
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return reject(http.StatusBadRequest, msgRejected, "invalid claims", err)
	}
	// Some providers (Authelia 4.39+ by default) keep the profile out of the ID
	// token and serve it from UserInfo: fill in what the ID token lacks.
	if (claims.Name == "" && claims.PreferredUsername == "") || claims.Email == "" {
		info, err := o.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		switch {
		case err != nil:
			slog.WarnContext(ctx, "oidc userinfo unavailable; using the ID token's claims", "err", err)
		case info.Subject != idToken.Subject: // OIDC Core 5.3.2: don't trust UserInfo about someone else
			slog.WarnContext(ctx, "oidc userinfo is for another subject; ignored")
		default:
			var more struct {
				Email             string `json:"email"`
				Name              string `json:"name"`
				PreferredUsername string `json:"preferred_username"`
			}
			if err := info.Claims(&more); err == nil {
				claims.Email = cmp.Or(claims.Email, more.Email)
				claims.Name = cmp.Or(claims.Name, more.Name)
				claims.PreferredUsername = cmp.Or(claims.PreferredUsername, more.PreferredUsername)
			}
		}
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	if name == "" {
		name = claims.Email
	}
	if name == "" {
		name = idToken.Subject
	}

	user, err := o.sessions.Store.UpsertOIDCUser(ctx, idToken.Issuer, idToken.Subject, claims.Email, name)
	if err != nil {
		slog.ErrorContext(ctx, "upsert user", "err", err)
		fail(w, r, LoginError{Status: http.StatusInternalServerError, Message: "Sign-in failed. Please try again.", Next: next})
		return "", false
	}
	if _, err := o.sessions.Start(ctx, w, user); err != nil {
		slog.ErrorContext(ctx, "start session", "err", err)
		fail(w, r, LoginError{Status: http.StatusInternalServerError, Message: "Sign-in failed. Please try again.", Next: next})
		return "", false
	}
	return fs.Next, true
}
