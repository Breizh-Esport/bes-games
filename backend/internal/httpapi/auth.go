package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/securecookie"
	"golang.org/x/oauth2"

	"github.com/valentin/bes-games/backend/internal/core"
)

const (
	backchannelLogoutEvent = "http://schemas.openid.net/event/backchannel-logout"
)

type authContextKey struct{}

type authInfo struct {
	Sub       string
	SessionID string
}

type AuthConfig struct {
	IssuerURL              string
	ClientID               string
	ClientSecret           string
	RedirectURL            string
	PublicURL              string
	UIBaseURL              string
	Scopes                 []string
	Prompt                 string
	OfflineAccess          bool
	CookieSecret           string
	CookieName             string
	CookieDomain           string
	CookieSecure           bool
	CookieSameSite         http.SameSite
	RefreshTokenTTL        time.Duration
	AccessTokenFallbackTTL time.Duration
	AllowLegacyHeader      bool
}

type AuthService struct {
	cfg      AuthConfig
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
	cookie   *securecookie.SecureCookie
	coreRepo *core.Repo
	now      func() time.Time
}

type authState struct {
	State        string
	Nonce        string
	CodeVerifier string
	ReturnTo     string
	CreatedAt    int64
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var info authInfo
		if s.auth == nil {
			info.Sub = strings.TrimSpace(r.Header.Get("X-User-Sub"))
		} else {
			info = s.auth.AuthenticateRequest(w, r)
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	s.auth.login(w, r)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	s.auth.callback(w, r)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.logout(w, r)
}

func (s *Server) handleBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.handleBackchannelLogout(w, r)
}

func NewAuthService(ctx context.Context, repo *core.Repo, cfg AuthConfig) (*AuthService, error) {
	if repo == nil {
		return nil, fmt.Errorf("auth requires core repo")
	}
	if cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil, fmt.Errorf("missing issuer url or client id")
	}
	if cfg.CookieSecret == "" {
		return nil, fmt.Errorf("missing auth cookie secret")
	}
	if cfg.CookieName == "" {
		cfg.CookieName = "besgames_session"
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if cfg.AccessTokenFallbackTTL == 0 {
		cfg.AccessTokenFallbackTTL = 5 * time.Minute
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	redirectURL := cfg.RedirectURL
	if redirectURL == "" && cfg.PublicURL != "" {
		redirectURL = strings.TrimRight(cfg.PublicURL, "/") + "/auth/callback"
	}
	if redirectURL == "" {
		return nil, fmt.Errorf("missing redirect url (set BES_PUBLIC_URL or BES_OIDC_REDIRECT_URL)")
	}

	oauth := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       cfg.Scopes,
	}

	hashKey, blockKey := deriveCookieKeys(cfg.CookieSecret)
	cookie := securecookie.New(hashKey, blockKey)

	return &AuthService{
		cfg:      cfg,
		provider: provider,
		verifier: verifier,
		oauth:    oauth,
		cookie:   cookie,
		coreRepo: repo,
		now:      time.Now,
	}, nil
}

func deriveCookieKeys(secret string) ([]byte, []byte) {
	seed := []byte(secret)
	hash := sha256.Sum256(append(seed, 0))
	block := sha256.Sum256(append(seed, 1))
	return hash[:], block[:]
}

func (a *AuthService) AuthenticateRequest(w http.ResponseWriter, r *http.Request) authInfo {
	if a == nil {
		return authInfo{}
	}

	sessionID := a.readSessionID(r)
	if sessionID == "" {
		if a.cfg.AllowLegacyHeader {
			sub := strings.TrimSpace(r.Header.Get("X-User-Sub"))
			return authInfo{Sub: sub}
		}
		return authInfo{}
	}

	sess, err := a.coreRepo.GetSession(r.Context(), sessionID)
	if err != nil {
		a.clearSessionCookie(w)
		return authInfo{}
	}

	now := a.now().UTC()
	if sess.RefreshExpiresAt.Before(now) {
		_ = a.coreRepo.RevokeSession(r.Context(), sess.ID)
		a.clearSessionCookie(w)
		return authInfo{}
	}

	if sess.AccessExpiresAt.Before(now.Add(30 * time.Second)) {
		if err := a.refreshSessionTokens(r.Context(), sess); err != nil {
			_ = a.coreRepo.RevokeSession(r.Context(), sess.ID)
			a.clearSessionCookie(w)
			return authInfo{}
		}
	}

	return authInfo{
		Sub:       sess.Sub,
		SessionID: sess.ID,
	}
}

func (a *AuthService) readSessionID(r *http.Request) string {
	c, err := r.Cookie(a.cfg.CookieName)
	if err != nil || c == nil {
		return ""
	}
	var sessionID string
	if err := a.cookie.Decode(a.cfg.CookieName, c.Value, &sessionID); err != nil {
		return ""
	}
	return sessionID
}

func (a *AuthService) setSessionCookie(w http.ResponseWriter, sessionID string) error {
	encoded, err := a.cookie.Encode(a.cfg.CookieName, sessionID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    encoded,
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: a.cfg.CookieSameSite,
		MaxAge:   int(a.cfg.RefreshTokenTTL.Seconds()),
	})
	return nil
}

func (a *AuthService) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: a.cfg.CookieSameSite,
		MaxAge:   -1,
	})
}

func (a *AuthService) setStateCookie(w http.ResponseWriter, st authState) error {
	const stateCookieName = "besgames_auth_state"
	encoded, err := a.cookie.Encode(stateCookieName, st)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    encoded,
		Path:     "/auth",
		Domain:   a.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: a.cfg.CookieSameSite,
		MaxAge:   300,
	})
	return nil
}

func (a *AuthService) readStateCookie(r *http.Request) (authState, bool) {
	const stateCookieName = "besgames_auth_state"
	c, err := r.Cookie(stateCookieName)
	if err != nil || c == nil {
		return authState{}, false
	}
	var st authState
	if err := a.cookie.Decode(stateCookieName, c.Value, &st); err != nil {
		return authState{}, false
	}
	return st, true
}

func (a *AuthService) clearStateCookie(w http.ResponseWriter) {
	const stateCookieName = "besgames_auth_state"
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/auth",
		Domain:   a.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: a.cfg.CookieSameSite,
		MaxAge:   -1,
	})
}

func (a *AuthService) sanitizeReturnTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if a.cfg.UIBaseURL != "" {
			return a.cfg.UIBaseURL
		}
		return "/"
	}

	if strings.HasPrefix(raw, "/") {
		if a.cfg.UIBaseURL != "" {
			return strings.TrimRight(a.cfg.UIBaseURL, "/") + raw
		}
		return raw
	}

	if a.cfg.UIBaseURL != "" {
		base, err := url.Parse(a.cfg.UIBaseURL)
		if err == nil {
			target, err := url.Parse(raw)
			if err == nil && sameOrigin(base, target) {
				return raw
			}
		}
		return a.cfg.UIBaseURL
	}

	return "/"
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	if !strings.EqualFold(a.Scheme, b.Scheme) {
		return false
	}
	return strings.EqualFold(a.Host, b.Host)
}

func (a *AuthService) beginAuth(w http.ResponseWriter, r *http.Request) error {
	state := randomToken()
	nonce := randomToken()
	codeVerifier := randomPKCEVerifier()

	st := authState{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ReturnTo:     a.sanitizeReturnTo(r.URL.Query().Get("returnTo")),
		CreatedAt:    a.now().UTC().Unix(),
	}
	if err := a.setStateCookie(w, st); err != nil {
		return fmt.Errorf("set auth state: %w", err)
	}

	challenge := pkceChallenge(codeVerifier)
	opts := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	}
	if a.cfg.OfflineAccess {
		opts = append(opts, oauth2.SetAuthURLParam("access_type", "offline"))
	}
	if a.cfg.Prompt != "" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", a.cfg.Prompt))
	}

	http.Redirect(w, r, a.oauth.AuthCodeURL(state, opts...), http.StatusFound)
	return nil
}

func (a *AuthService) finishAuth(w http.ResponseWriter, r *http.Request) (string, error) {
	query := r.URL.Query()
	if errStr := strings.TrimSpace(query.Get("error")); errStr != "" {
		return "", fmt.Errorf("provider error: %s", errStr)
	}
	code := strings.TrimSpace(query.Get("code"))
	state := strings.TrimSpace(query.Get("state"))
	if code == "" || state == "" {
		return "", fmt.Errorf("missing code or state")
	}

	st, ok := a.readStateCookie(r)
	if !ok || st.State == "" || st.State != state {
		return "", fmt.Errorf("invalid auth state")
	}
	a.clearStateCookie(w)

	token, err := a.oauth.Exchange(r.Context(), code, oauth2.SetAuthURLParam("code_verifier", st.CodeVerifier))
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "", fmt.Errorf("missing id_token")
	}

	idToken, err := a.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		return "", fmt.Errorf("verify id_token: %w", err)
	}

	var claims struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Picture           string `json:"picture"`
		SID               string `json:"sid"`
		Nonce             string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", fmt.Errorf("id_token claims: %w", err)
	}
	if claims.Nonce != "" && claims.Nonce != st.Nonce {
		return "", fmt.Errorf("nonce mismatch")
	}
	if claims.Sub == "" {
		return "", fmt.Errorf("missing sub")
	}

	nickname := strings.TrimSpace(claims.PreferredUsername)
	if nickname == "" {
		nickname = strings.TrimSpace(claims.Name)
	}
	if nickname == "" && claims.Email != "" {
		if at := strings.Index(claims.Email, "@"); at > 0 {
			nickname = claims.Email[:at]
		}
	}
	if nickname == "" {
		nickname = "Player"
	}

	if _, err := a.coreRepo.UpsertProfile(r.Context(), claims.Sub, nickname, strings.TrimSpace(claims.Picture)); err != nil {
		return "", fmt.Errorf("upsert profile: %w", err)
	}

	accessExpiry := token.Expiry
	if accessExpiry.IsZero() {
		accessExpiry = a.now().Add(a.cfg.AccessTokenFallbackTTL)
	}

	refreshExpiresAt := accessExpiry
	if token.RefreshToken != "" {
		refreshExpiresAt = a.now().Add(a.cfg.RefreshTokenTTL)
	}

	sess, err := a.coreRepo.CreateSession(r.Context(), core.UserSession{
		Sub:              claims.Sub,
		SID:              strings.TrimSpace(claims.SID),
		RefreshToken:     token.RefreshToken,
		AccessToken:      token.AccessToken,
		IDToken:          rawIDToken,
		AccessExpiresAt:  accessExpiry,
		RefreshExpiresAt: refreshExpiresAt,
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	if err := a.setSessionCookie(w, sess.ID); err != nil {
		return "", fmt.Errorf("set session cookie: %w", err)
	}

	return st.ReturnTo, nil
}

func (a *AuthService) refreshSessionTokens(ctx context.Context, sess core.UserSession) error {
	if sess.RefreshToken == "" {
		return fmt.Errorf("missing refresh token")
	}

	src := a.oauth.TokenSource(ctx, &oauth2.Token{RefreshToken: sess.RefreshToken})
	token, err := src.Token()
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}

	newAccess := token.AccessToken
	newIDToken := sess.IDToken
	refreshToken := sess.RefreshToken
	refreshExpiresAt := sess.RefreshExpiresAt

	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" {
		idToken, err := a.verifier.Verify(ctx, rawIDToken)
		if err != nil {
			return fmt.Errorf("verify refreshed id_token: %w", err)
		}
		var claims struct {
			Sub string `json:"sub"`
		}
		if err := idToken.Claims(&claims); err != nil {
			return fmt.Errorf("refreshed id_token claims: %w", err)
		}
		if claims.Sub != "" && claims.Sub != sess.Sub {
			return fmt.Errorf("sub mismatch on refresh")
		}
		newIDToken = rawIDToken
	}

	if token.RefreshToken != "" {
		refreshToken = token.RefreshToken
		refreshExpiresAt = a.now().Add(a.cfg.RefreshTokenTTL)
	}

	accessExpiry := token.Expiry
	if accessExpiry.IsZero() {
		accessExpiry = a.now().Add(a.cfg.AccessTokenFallbackTTL)
	}

	return a.coreRepo.UpdateSessionTokens(
		ctx,
		sess.ID,
		newAccess,
		newIDToken,
		accessExpiry,
		&refreshToken,
		&refreshExpiresAt,
	)
}

func randomPKCEVerifier() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return randomToken()
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (a *AuthService) handleBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	logoutToken, err := readLogoutToken(r)
	if err != nil || logoutToken == "" {
		writeError(w, http.StatusBadRequest, "invalid logout token")
		return
	}

	idToken, err := a.verifier.Verify(r.Context(), logoutToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid logout token")
		return
	}

	var claims struct {
		Sub    string                 `json:"sub"`
		SID    string                 `json:"sid"`
		Events map[string]interface{} `json:"events"`
	}
	if err := idToken.Claims(&claims); err != nil {
		writeError(w, http.StatusBadRequest, "invalid logout token")
		return
	}

	if claims.Events == nil || claims.Events[backchannelLogoutEvent] == nil {
		writeError(w, http.StatusBadRequest, "invalid logout token")
		return
	}

	if claims.SID != "" {
		_ = a.coreRepo.RevokeSessionsBySID(r.Context(), claims.SID)
	} else if claims.Sub != "" {
		_ = a.coreRepo.RevokeSessionsBySub(r.Context(), claims.Sub)
	} else {
		writeError(w, http.StatusBadRequest, "invalid logout token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func readLogoutToken(r *http.Request) (string, error) {
	ct := strings.ToLower(r.Header.Get("content-type"))
	if strings.Contains(ct, "application/json") {
		var payload struct {
			LogoutToken string `json:"logout_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return "", err
		}
		return strings.TrimSpace(payload.LogoutToken), nil
	}

	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.FormValue("logout_token")), nil
}

func (a *AuthService) logout(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	sessionID := a.readSessionID(r)
	if sessionID != "" {
		_ = a.coreRepo.RevokeSession(r.Context(), sessionID)
	}
	a.clearSessionCookie(w)

	returnTo := a.sanitizeReturnTo(r.URL.Query().Get("returnTo"))
	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (a *AuthService) callback(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	returnTo, err := a.finishAuth(w, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}

	http.Redirect(w, r, returnTo, http.StatusFound)
}

func (a *AuthService) login(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err := a.beginAuth(w, r); err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
	}
}
