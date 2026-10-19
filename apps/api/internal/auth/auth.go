package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "repotrial_session"
	csrfCookie    = "repotrial_csrf"
	oauthCookie   = "repotrial_oauth"
	stateLifetime = 10 * time.Minute
	sessionLife   = 7 * 24 * time.Hour
)

type Config struct {
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	AppURL           string
	StateSecret      string
	CookieSecure     bool
	AuthorizationURL string
	TokenURL         string
	UserInfoURL      string
	HTTPClient       *http.Client
}

type Handler struct {
	repo      Repository
	config    Config
	client    *http.Client
	appOrigin string
}

func NewHandler(repo Repository, cfg Config) (*Handler, error) {
	if repo == nil || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.StateSecret == "" {
		return nil, errors.New("OAuth repository, client credentials, and state secret are required")
	}
	redirect, err := url.Parse(cfg.RedirectURL)
	if err != nil || redirect.Host == "" || (redirect.Scheme != "https" && redirect.Scheme != "http") {
		return nil, errors.New("invalid OAuth redirect URL")
	}
	app, err := url.Parse(cfg.AppURL)
	if err != nil || app.Host == "" || (app.Scheme != "https" && app.Scheme != "http") {
		return nil, errors.New("invalid application URL")
	}
	if cfg.CookieSecure && (redirect.Scheme != "https" || app.Scheme != "https") {
		return nil, errors.New("secure OAuth deployment requires HTTPS URLs")
	}
	if cfg.AuthorizationURL == "" {
		cfg.AuthorizationURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if cfg.UserInfoURL == "" {
		cfg.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Handler{
		repo:      repo,
		config:    cfg,
		client:    client,
		appOrigin: app.Scheme + "://" + app.Host,
	}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/google/start", h.start)
	mux.HandleFunc("GET /api/v1/auth/google/callback", h.callback)
	mux.Handle("GET /api/v1/auth/session", h.Authenticate(http.HandlerFunc(h.session)))
	mux.Handle("DELETE /api/v1/auth/session", h.Authenticate(http.HandlerFunc(h.logout)))
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not start sign-in")
		return
	}
	verifier, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not start sign-in")
		return
	}
	expiry := time.Now().Add(stateLifetime)
	value := signState(state, verifier, expiry.Unix(), h.config.StateSecret)
	h.setCookie(w, oauthCookie, value, int(stateLifetime.Seconds()), true, "/api/v1/auth/google")
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"client_id":             {h.config.ClientID},
		"redirect_uri":          {h.config.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, h.config.AuthorizationURL+"?"+values.Encode(), http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(oauthCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_oauth_state", "sign-in session expired")
		return
	}
	h.setCookie(w, oauthCookie, "", -1, true, "/api/v1/auth/google")
	state, verifier, ok := verifyState(cookie.Value, h.config.StateSecret, time.Now())
	provided := r.URL.Query().Get("state")
	if !ok || provided == "" || subtle.ConstantTimeCompare([]byte(state), []byte(provided)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_oauth_state", "invalid sign-in state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || r.URL.Query().Get("error") != "" {
		writeError(w, http.StatusBadRequest, "oauth_failed", "Google sign-in was not completed")
		return
	}
	accessToken, err := h.exchangeCode(r.Context(), code, verifier)
	if err != nil {
		writeError(w, http.StatusBadGateway, "oauth_failed", "could not complete Google sign-in")
		return
	}
	identity, err := h.fetchIdentity(r.Context(), accessToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "oauth_failed", "could not verify Google identity")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create session")
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create session")
		return
	}
	_, err = h.repo.CreateSession(
		r.Context(),
		identity,
		sha256.Sum256([]byte(token)),
		sha256.Sum256([]byte(csrf)),
		time.Now().Add(sessionLife),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create session")
		return
	}
	h.setCookie(w, sessionCookie, token, int(sessionLife.Seconds()), true, "/")
	h.setCookie(w, csrfCookie, csrf, int(sessionLife.Seconds()), false, "/")
	http.Redirect(w, r, h.config.AppURL, http.StatusFound)
}

func (h *Handler) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"client_id":     {h.config.ClientID},
		"client_secret": {h.config.ClientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {h.config.RedirectURL},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint status %d", res.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" || !strings.EqualFold(payload.TokenType, "Bearer") {
		return "", errors.New("invalid token response")
	}
	return payload.AccessToken, nil
}

func (h *Handler) fetchIdentity(ctx context.Context, token string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.config.UserInfoURL, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := h.client.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("userinfo endpoint status %d", res.StatusCode)
	}
	var profile struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&profile); err != nil {
		return Identity{}, err
	}
	if profile.Subject == "" || profile.Email == "" || !profile.EmailVerified {
		return Identity{}, errors.New("unverified identity")
	}
	name := profile.Name
	if name == "" {
		name = profile.Email
	}
	var avatar *string
	if profile.Picture != "" {
		avatar = &profile.Picture
	}
	return Identity{Subject: profile.Subject, Email: profile.Email, DisplayName: name, AvatarURL: avatar}, nil
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func signState(state, verifier string, expiry int64, secret string) string {
	data := state + "." + verifier + "." + strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(data))
	return data + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyState(value, secret string, now time.Time) (string, string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return "", "", false
	}
	expiry, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() >= expiry || expiry > now.Add(stateLifetime).Unix() {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return "", "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join(parts[:3], ".")))
	if !hmac.Equal(decoded, mac.Sum(nil)) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (h *Handler) setCookie(w http.ResponseWriter, name, value string, maxAge int, httpOnly bool, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: httpOnly,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message})
}
