package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const flowCookie = "onerep_oidc"

// OIDC implements the authorization code flow with PKCE.
type OIDC struct {
	sessions *Sessions
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	issuer   string
}

// NewOIDC discovers the provider at issuer. baseURL is the public URL of onerep.
func NewOIDC(ctx context.Context, issuer, clientID, clientSecret, baseURL string, sessions *Sessions) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	return &OIDC{
		sessions: sessions,
		issuer:   issuer,
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
	fs := flowState{State: state, Nonce: nonce, Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
	raw, _ := json.Marshal(fs)
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookie,
		Value:    base64.RawURLEncoding.EncodeToString(raw),
		Path:     "/auth/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   o.sessions.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(fs.Verifier)), http.StatusFound)
}

func (o *OIDC) readFlow(r *http.Request) (flowState, error) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		return flowState{}, errors.New("missing login state cookie")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return flowState{}, err
	}
	var fs flowState
	return fs, json.Unmarshal(raw, &fs)
}

// Callback completes the login, creating the user on first login.
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	fail := func(msg string, err error) {
		slog.WarnContext(ctx, "oidc callback failed", "reason", msg, "err", err)
		http.Error(w, "login failed: "+msg, http.StatusBadRequest)
	}

	if e := r.URL.Query().Get("error"); e != "" {
		fail("identity provider returned "+e, nil)
		return
	}
	fs, err := o.readFlow(r)
	if err != nil {
		fail("invalid login state", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Value: "", Path: "/auth/", MaxAge: -1})
	if r.URL.Query().Get("state") != fs.State {
		fail("state mismatch", nil)
		return
	}

	token, err := o.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(fs.Verifier))
	if err != nil {
		fail("code exchange", err)
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok {
		fail("no id_token in token response", nil)
		return
	}
	idToken, err := o.verifier.Verify(ctx, rawID)
	if err != nil {
		fail("invalid id_token", err)
		return
	}
	if idToken.Nonce != fs.Nonce {
		fail("nonce mismatch", nil)
		return
	}
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		fail("invalid claims", err)
		return
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
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	if _, err := o.sessions.Start(ctx, w, user); err != nil {
		slog.ErrorContext(ctx, "start session", "err", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fs.Next, http.StatusSeeOther)
}
