package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mapherez/nox-sync/backend/internal/storage"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
)

const webSessionDuration = 14 * 24 * time.Hour

func (s *Server) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	if !s.googleOAuthConfigured() {
		writeJSONError(w, http.StatusInternalServerError, "OAUTH_NOT_CONFIGURED", "Google OAuth is not configured.")
		return
	}

	redirectTo := r.URL.Query().Get("redirect")
	state, err := s.store.CreateOAuthState(r.Context(), redirectTo)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to create OAuth state.")
		return
	}

	http.Redirect(w, r, s.googleOAuthConfig(r).AuthCodeURL(state), http.StatusSeeOther)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	if !s.googleOAuthConfigured() {
		writeJSONError(w, http.StatusInternalServerError, "OAUTH_NOT_CONFIGURED", "Google OAuth is not configured.")
		return
	}
	if errMessage := strings.TrimSpace(r.URL.Query().Get("error")); errMessage != "" {
		writeJSONError(w, http.StatusForbidden, "OAUTH_FAILED", "Google login was cancelled or rejected.")
		return
	}

	redirectTo, err := s.store.ConsumeOAuthState(r.Context(), r.URL.Query().Get("state"))
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "OAUTH_STATE_INVALID", "Google login state is invalid or expired.")
		return
	}

	token, err := s.googleOAuthConfig(r).Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "OAUTH_EXCHANGE_FAILED", "Failed to exchange Google login code.")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		writeJSONError(w, http.StatusForbidden, "OAUTH_ID_TOKEN_MISSING", "Google did not return an ID token.")
		return
	}

	payload, err := idtoken.Validate(r.Context(), rawIDToken, s.cfg.GoogleClientID)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "OAUTH_ID_TOKEN_INVALID", "Google ID token is invalid.")
		return
	}

	profile, err := profileFromIDToken(payload)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "OAUTH_PROFILE_INVALID", err.Error())
		return
	}

	user, err := s.store.CompleteGoogleLogin(r.Context(), profile)
	if err != nil {
		if errors.Is(err, storage.ErrForbidden) {
			writeJSONError(w, http.StatusForbidden, "USER_NOT_ALLOWED", "This Google account is not allowed to use this NoX Sync server.")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to complete login.")
		return
	}

	sessionToken, err := s.store.CreateWebSession(r.Context(), user.ID, webSessionDuration)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to create web session.")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(webSessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	})
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteWebSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
}

func (s *Server) googleOAuthConfigured() bool {
	return strings.TrimSpace(s.cfg.GoogleClientID) != "" && strings.TrimSpace(s.cfg.GoogleClientSecret) != ""
}

func (s *Server) googleOAuthConfig(r *http.Request) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.GoogleClientID,
		ClientSecret: s.cfg.GoogleClientSecret,
		RedirectURL:  s.publicURL(r) + "/auth/google/callback",
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     google.Endpoint,
	}
}

func profileFromIDToken(payload *idtoken.Payload) (storage.OAuthProfile, error) {
	sub := strings.TrimSpace(payload.Subject)
	email, _ := stringClaim(payload.Claims, "email")
	if sub == "" || email == "" {
		return storage.OAuthProfile{}, fmt.Errorf("Google identity is missing sub or email.")
	}
	if verified, ok := boolClaim(payload.Claims, "email_verified"); ok && !verified {
		return storage.OAuthProfile{}, fmt.Errorf("Google email is not verified.")
	}
	firstName, _ := stringClaim(payload.Claims, "given_name")
	displayName, _ := stringClaim(payload.Claims, "name")
	return storage.OAuthProfile{
		Sub:         sub,
		Email:       email,
		FirstName:   firstName,
		DisplayName: displayName,
	}, nil
}

func stringClaim(claims map[string]any, key string) (string, bool) {
	value, ok := claims[key].(string)
	return strings.TrimSpace(value), ok
}

func boolClaim(claims map[string]any, key string) (bool, bool) {
	value, ok := claims[key].(bool)
	return value, ok
}
