package desktop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const Provider = "gemini-desktop"
const Model = "gemini-desktop-auto"
const authEndpoint = "https://accounts.google.com/o/oauth2/auth"
const tokenEndpoint = "https://www.googleapis.com/oauth2/v4/token"
const userinfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo"
const flexibleScope = "https://www.googleapis.com/auth/flexible-api"
const tokenScopes = "https://www.googleapis.com/auth/assistant https://www.googleapis.com/auth/gemini https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/gdm-genesis-janus.firstparty"

type Credential struct {
	Type         string    `json:"type"`
	Email        string    `json:"email,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	ProxyURL     string    `json:"proxy_url,omitempty"`
}

type Login struct {
	URL      string
	State    string
	Verifier string
	Redirect string
	Expires  time.Time
}

type StatusError struct {
	Status  int
	Message string
	Cause   error
}

func (e *StatusError) Error() string           { return e.Message }
func (e *StatusError) StatusCode() int         { return e.Status }
func (e *StatusError) Unwrap() error           { return e.Cause }
func failure(status int, message string) error { return &StatusError{Status: status, Message: message} }

func Start(redirect string) (Login, error) {
	u, err := url.Parse(redirect)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || u.Port() == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return Login{}, failure(400, "Gemini Desktop requires an HTTP loopback callback with a port")
	}
	stateBytes := make([]byte, 32)
	verifierBytes := make([]byte, 48)
	if _, err = rand.Read(stateBytes); err != nil {
		return Login{}, err
	}
	if _, err = rand.Read(verifierBytes); err != nil {
		return Login{}, err
	}
	l := Login{State: base64.RawURLEncoding.EncodeToString(stateBytes), Verifier: base64.RawURLEncoding.EncodeToString(verifierBytes), Redirect: redirect, Expires: time.Now().Add(10 * time.Minute)}
	hash := sha256.Sum256([]byte(l.Verifier))
	q := url.Values{"client_id": {nativeClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {flexibleScope}, "access_type": {"offline"}, "state": {l.State}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}}
	l.URL = authEndpoint + "?" + q.Encode()
	return l, nil
}

func Exchange(ctx context.Context, client pluginapi.HostHTTPClient, l Login, code, state string) (Credential, error) {
	if state != l.State || state == "" || time.Now().After(l.Expires) {
		return Credential{}, failure(400, "Invalid or expired Gemini login session")
	}
	if strings.TrimSpace(code) == "" {
		return Credential{}, failure(400, "Authorization code is missing")
	}
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {l.Redirect}, "code_verifier": {l.Verifier}}
	c, err := token(ctx, client, values, Credential{})
	if err != nil {
		return Credential{}, err
	}
	if c.RefreshToken == "" {
		return Credential{}, failure(401, "Gemini did not return a refresh token; complete a new login")
	}
	// The native authorization grants flexible scopes. Mint a service-scoped token
	// before model calls, using the same native refresh grant.
	c, err = Refresh(ctx, client, c)
	if err != nil {
		return Credential{}, err
	}
	if c.Email == "" {
		return Credential{}, failure(502, "Could not identify the Google account for this new login")
	}
	return c, nil
}

func Refresh(ctx context.Context, client pluginapi.HostHTTPClient, c Credential) (Credential, error) {
	if c.RefreshToken == "" {
		return Credential{}, failure(401, "Gemini refresh token is missing")
	}
	c, err := token(ctx, client, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}, "scope": {tokenScopes}}, c)
	if err != nil {
		return Credential{}, err
	}
	if c.Email == "" {
		identified, errIdentify := identify(ctx, client, c)
		if errIdentify == nil {
			return identified, nil
		}
		// Account labeling must not discard a successful token refresh. In
		// particular, a rotated refresh token must reach the host for persistence.
		// Legacy credentials can acquire the verified email on a later refresh.
	}
	return c, nil
}

func identify(ctx context.Context, client pluginapi.HostHTTPClient, c Credential) (Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, err := client.Do(ctx, pluginapi.HTTPRequest{Method: http.MethodGet, URL: userinfoEndpoint, Headers: http.Header{"Authorization": {"Bearer " + c.AccessToken}}})
	if err != nil || response.StatusCode != http.StatusOK {
		return Credential{}, failure(502, "Could not retrieve the Google account email")
	}
	var profile struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified_email"`
	}
	if len(response.Body) > 64<<10 || json.Unmarshal(response.Body, &profile) != nil {
		return Credential{}, failure(502, "Invalid Google account profile")
	}
	address, err := mail.ParseAddress(profile.Email)
	if err != nil || address.Address != profile.Email || !profile.Verified || strings.ContainsAny(profile.Email, "/\\\r\n\x00") {
		return Credential{}, failure(502, "Google did not return a verified account email")
	}
	c.Email = profile.Email
	return c, nil
}

func token(ctx context.Context, client pluginapi.HostHTTPClient, values url.Values, old Credential) (Credential, error) {
	if client == nil {
		return Credential{}, failure(500, "Host HTTP client is unavailable")
	}
	values.Set("client_id", nativeClientID)
	values.Set("client_secret", nativeClientSecret())
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := client.Do(ctx, pluginapi.HTTPRequest{Method: http.MethodPost, URL: tokenEndpoint, Headers: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, Body: []byte(values.Encode())})
	if err != nil {
		return Credential{}, failure(502, "Could not reach Google's token service")
	}
	var t struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if err = json.Unmarshal(response.Body, &t); err != nil {
		return Credential{}, failure(502, "Invalid response from Google's token service")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := response.StatusCode
		if t.Error == "invalid_grant" || t.Error == "invalid_client" {
			status = 401
		}
		if status < 400 || status > 599 {
			status = 502
		}
		// Never forward the raw response body: it may contain account information.
		reason := "token request rejected"
		for _, allowed := range []string{"invalid_grant", "invalid_client", "invalid_scope", "access_denied", "invalid_request"} {
			if t.Error == allowed {
				reason = allowed
			}
		}
		return Credential{}, failure(status, "Google OAuth: "+reason)
	}
	if t.AccessToken == "" || t.ExpiresIn <= 0 {
		return Credential{}, failure(502, "Google returned an incomplete token response")
	}
	if t.RefreshToken != "" {
		old.RefreshToken = t.RefreshToken
	}
	old.Type, old.AccessToken, old.ExpiresAt, old.Scope = Provider, t.AccessToken, time.Now().UTC().Add(time.Duration(t.ExpiresIn)*time.Second), t.Scope
	return old, nil
}

func ParseCredential(raw []byte) (Credential, error) {
	var c Credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, failure(400, "Invalid Gemini credential JSON")
	}
	if c.Type != Provider {
		return c, failure(400, "Credential belongs to a different provider")
	}
	if c.RefreshToken == "" {
		return c, failure(401, "Gemini credential has no refresh token")
	}
	return c, nil
}

func Auth(c Credential) (pluginapi.AuthData, error) {
	if c.Type != Provider || c.RefreshToken == "" {
		return pluginapi.AuthData{}, failure(400, "Invalid Gemini Desktop credential")
	}
	if c.Email != "" {
		address, e := mail.ParseAddress(c.Email)
		if e != nil || address.Address != c.Email || strings.ContainsAny(c.Email, "/\\\r\n\x00") {
			return pluginapi.AuthData{}, failure(400, "Invalid Gemini account email")
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return pluginapi.AuthData{}, err
	}
	hash := sha256.Sum256([]byte(c.RefreshToken))
	name := Provider + "-" + hex.EncodeToString(hash[:8]) + ".json"
	label := "Gemini Desktop"
	if c.Email != "" {
		label = c.Email
		name = Provider + "-" + c.Email + ".json"
	}
	next := c.ExpiresAt.Add(-5 * time.Minute)
	return pluginapi.AuthData{Provider: Provider, ID: name, FileName: name, Label: label, ProxyURL: c.ProxyURL, StorageJSON: raw, Metadata: map[string]any{"type": Provider, "email": c.Email, "access_token": c.AccessToken, "refresh_token": c.RefreshToken, "expired": c.ExpiresAt.Format(time.RFC3339)}, NextRefreshAfter: next}, nil
}

func errorf(status int, format string, args ...any) error {
	return failure(status, fmt.Sprintf(format, args...))
}
