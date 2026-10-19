package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type memoryRepo struct {
	identity Identity
	token    [32]byte
	csrf     [32]byte
	revoked  bool
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func providerResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func (r *memoryRepo) CreateSession(_ context.Context, identity Identity, token, csrf [32]byte, _ time.Time) (User, error) {
	r.identity = identity
	r.token = token
	r.csrf = csrf
	return User{ID: "00000000-0000-0000-0000-000000000001", DisplayName: identity.DisplayName}, nil
}

func (r *memoryRepo) GetSession(_ context.Context, token [32]byte) (User, [32]byte, error) {
	if r.revoked || token != r.token {
		return User{}, [32]byte{}, ErrUnauthenticated
	}
	return User{ID: "00000000-0000-0000-0000-000000000001", DisplayName: r.identity.DisplayName}, r.csrf, nil
}

func (r *memoryRepo) RevokeSession(_ context.Context, token [32]byte) error {
	if token != r.token {
		return ErrUnauthenticated
	}
	r.revoked = true
	return nil
}

func TestGoogleFlowAndCSRFBoundary(t *testing.T) {
	var receivedVerifier string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			receivedVerifier = r.Form.Get("code_verifier")
			return providerResponse(`{"access_token":"access","token_type":"Bearer"}`), nil
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("userinfo missing access token")
			}
			return providerResponse(`{"sub":"google-subject","email":"user@example.com","email_verified":true,"name":"User"}`), nil
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			return providerResponse(`{}`), nil
		}
	})}

	repo := &memoryRepo{}
	h, err := NewHandler(repo, Config{
		ClientID: "client", ClientSecret: "secret", RedirectURL: "http://app.test/api/v1/auth/google/callback",
		AppURL: "http://app.test", StateSecret: "state-secret", AuthorizationURL: "https://provider.test/authorize",
		TokenURL: "https://provider.test/token", UserInfoURL: "https://provider.test/userinfo", HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)

	start := httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/start", nil))
	if start.Code != http.StatusFound {
		t.Fatalf("start status %d", start.Code)
	}
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("code_challenge_method") != "S256" || location.Query().Get("state") == "" {
		t.Fatal("OAuth redirect missing state or PKCE")
	}
	stateCookie := start.Result().Cookies()[0]

	bad := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?state=attacker&code=code", nil)
	badReq.AddCookie(stateCookie)
	mux.ServeHTTP(bad, badReq)
	if bad.Code != http.StatusBadRequest || repo.identity.Subject != "" {
		t.Fatal("state mismatch created a session")
	}

	callback := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/auth/google/callback?state="+location.Query().Get("state")+"&code=code",
		nil,
	)
	callbackReq.AddCookie(stateCookie)
	mux.ServeHTTP(callback, callbackReq)
	if callback.Code != http.StatusFound || receivedVerifier == "" || repo.identity.Subject != "google-subject" {
		t.Fatalf("callback failed: %d", callback.Code)
	}
	challenge := sha256.Sum256([]byte(receivedVerifier))
	if location.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Fatal("token exchange verifier does not match authorization challenge")
	}
	var session, csrf *http.Cookie
	for _, cookie := range callback.Result().Cookies() {
		switch cookie.Name {
		case sessionCookie:
			session = cookie
		case csrfCookie:
			csrf = cookie
		}
	}
	if session == nil || csrf == nil || !session.HttpOnly || csrf.HttpOnly {
		t.Fatal("session or CSRF cookie attributes invalid")
	}
	if repo.token != sha256.Sum256([]byte(session.Value)) {
		t.Fatal("session token was not hashed")
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	getReq.AddCookie(session)
	mux.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"displayName":"User"`) {
		t.Fatalf("session lookup failed: %d %s", get.Code, get.Body.String())
	}

	logout := func(origin, header string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/session", nil)
		r.AddCookie(session)
		r.AddCookie(csrf)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", header)
		mux.ServeHTTP(w, r)
		return w.Code
	}
	if logout("http://evil.test", csrf.Value) != http.StatusForbidden || repo.revoked {
		t.Fatal("cross-origin logout was accepted")
	}
	if logout("http://app.test", "wrong") != http.StatusForbidden || repo.revoked {
		t.Fatal("invalid CSRF token was accepted")
	}
	if logout("http://app.test", csrf.Value) != http.StatusNoContent || !repo.revoked {
		t.Fatal("valid logout failed")
	}
}

func TestUnverifiedGoogleIdentityIsRejected(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return providerResponse(`{"sub":"subject","email":"user@example.com","email_verified":false}`), nil
	})}
	h, err := NewHandler(&memoryRepo{}, Config{
		ClientID: "client", ClientSecret: "secret", RedirectURL: "http://app.test/callback",
		AppURL: "http://app.test", StateSecret: "secret", UserInfoURL: "https://provider.test/userinfo", HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.fetchIdentity(context.Background(), "token"); err == nil {
		t.Fatal("unverified email was accepted")
	}
}
