package transport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/leetun2k2/go-auth/config"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

const (
	accessCookie  = "go_auth_access"
	refreshCookie = "go_auth_refresh"
	flowCookie    = "go_auth_flow"
	csrfCookie    = "go_auth_csrf"
	flowTTL       = 10 * time.Minute
	csrfTTL       = 10 * time.Minute
)

type browserAuth struct {
	config config.BrowserConfig
	gcm    cipher.AEAD
	key    []byte
}

type browserFlow struct {
	State     string `json:"state"`
	Verifier  string `json:"verifier"`
	Origin    string `json:"origin,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
}

func newBrowserAuth(cfg *config.BrowserConfig) (*browserAuth, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" || u.String() != cfg.Origin {
		return nil, errors.New("browser.origin must be an exact HTTP(S) origin without a path")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("browser.origin requires HTTP(S)")
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	local := host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (ip != nil && ip.IsLoopback())
	if u.Scheme == "http" && (!cfg.AllowInsecureHTTP || cfg.Secure || !local) {
		return nil, errors.New("HTTP browser origin requires explicit insecure local configuration")
	}
	if u.Scheme == "https" && !cfg.Secure {
		return nil, errors.New("HTTPS browser origin requires Secure cookies")
	}
	if cfg.Provider != "google" && cfg.Provider != "github" {
		return nil, errors.New("browser.provider must be google or github")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("browser client credentials are required")
	}
	key, err := base64.StdEncoding.DecodeString(cfg.CookieKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("browser.cookie_key must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("initialize browser cookie cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("initialize browser cookie encryption")
	}
	return &browserAuth{config: *cfg, gcm: gcm, key: key}, nil
}

func (b *browserAuth) credentials() credential.Credentials {
	return credential.Credentials{ClientID: b.config.ClientID, ClientSecret: b.config.ClientSecret}
}

func (b *browserAuth) matchesOrigin(origin string) bool {
	if origin == b.config.Origin {
		return true
	}
	if b.config.AllowInsecureHTTP {
		switch origin {
		case "http://localhost", "http://localhost:80", "http://localhost:3000", "http://admin.anlew.local", "http://127.0.0.1", "http://127.0.0.1:80", "http://127.0.0.1:3000":
			return true
		}
	}
	return false
}

func browserRequest(r *http.Request) bool {
	if r.Header.Get("Origin") != "" || r.Header.Get("X-CSRF-Token") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
		return true
	}
	for _, name := range []string{accessCookie, refreshCookie, flowCookie, csrfCookie} {
		if _, err := r.Cookie(name); err == nil {
			return true
		}
	}
	return false
}

// browserBoundary converts cookie credentials only after validating the browser
// boundary. It covers the entire admin prefix, including separately registered routes.
func (app *Application) browserBoundary(c *gin.Context) {
	path := c.Request.URL.Path
	if !strings.HasPrefix(path, "/api/auth/v1/") && !strings.HasPrefix(path, "/api/admin/v1/") && path != "/auth/callback" {
		c.Next()
		return
	}
	if !browserRequest(c.Request) {
		c.Next()
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	if origin := c.GetHeader("Origin"); origin != "" && !app.Browser.matchesOrigin(origin) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_failed"})
		return
	}
	for _, name := range []string{"Authorization", "X-Client-Id", "X-Client-Secret", "X-Client-Provider"} {
		if c.GetHeader(name) != "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}
	if path == "/api/auth/v1/exchange" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "browser_token_endpoint_forbidden"})
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
		if !app.Browser.validCSRF(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_failed"})
			return
		}
	}
	c.Set("browser_auth", true)
	c.Request.Header.Set("X-Client-Id", app.Browser.config.ClientID)
	c.Request.Header.Set("X-Client-Secret", app.Browser.config.ClientSecret)
	c.Request.Header.Set("X-Client-Provider", app.Browser.config.Provider)
	if token, err := c.Cookie(accessCookie); err == nil {
		c.Request.Header.Set("Authorization", "Bearer "+token)
	}
	c.Next()
}

func (b *browserAuth) setCookie(c *gin.Context, name, value string, maxAge int) {
	expires := time.Now().Add(time.Duration(maxAge) * time.Second)
	if maxAge < 0 {
		expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge,
		Expires: expires, HttpOnly: true, Secure: b.config.Secure, SameSite: http.SameSiteLaxMode})
}

func (b *browserAuth) clearSession(c *gin.Context) {
	for _, name := range []string{accessCookie, refreshCookie, flowCookie} {
		b.setCookie(c, name, "", -1)
	}
}

func (b *browserAuth) setSession(c *gin.Context, access, refresh string, accessTTL, refreshTTL int64) {
	b.setCookie(c, accessCookie, access, max(1, int(accessTTL)))
	b.setCookie(c, refreshCookie, refresh, max(1, int(refreshTTL)))
}

func randomBrowserValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (b *browserAuth) signCSRF(payload string) string {
	mac := hmac.New(sha256.New, b.key)
	_, _ = mac.Write([]byte("go-auth/csrf/" + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (b *browserAuth) csrfExpiry(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[0]) != 43 {
		return 0, false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expires <= time.Now().Unix() || expires > time.Now().Add(csrfTTL).Unix()+1 {
		return 0, false
	}
	want := b.signCSRF(parts[0] + "." + parts[1])
	return expires, subtle.ConstantTimeCompare([]byte(want), []byte(parts[2])) == 1
}

func (b *browserAuth) validCSRF(c *gin.Context) bool {
	if !b.matchesOrigin(c.GetHeader("Origin")) {
		return false
	}
	cookie, err := c.Cookie(csrfCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie), []byte(c.GetHeader("X-CSRF-Token"))) != 1 {
		return false
	}
	_, valid := b.csrfExpiry(cookie)
	return valid
}

func (app *Application) browserCSRF(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	origin := c.GetHeader("Origin")
	if origin != "" && !app.Browser.matchesOrigin(origin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "csrf_failed"})
		return
	}
	token, _ := c.Cookie(csrfCookie)
	expires, valid := app.Browser.csrfExpiry(token)
	if !valid {
		random, err := randomBrowserValue()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
			return
		}
		expires = time.Now().Add(csrfTTL).Unix()
		payload := random + "." + strconv.FormatInt(expires, 10)
		token = payload + "." + app.Browser.signCSRF(payload)
		app.Browser.setCookie(c, csrfCookie, token, int(csrfTTL/time.Second))
	}
	c.JSON(http.StatusOK, gin.H{"csrf_token": token, "expires_in": max(1, expires-time.Now().Unix())})
}

func (b *browserAuth) encodeFlow(flow browserFlow) (string, error) {
	data, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.gcm.Seal(nonce, nonce, data, []byte("go-auth/browser-flow/v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (b *browserAuth) decodeFlow(value string) (browserFlow, error) {
	var flow browserFlow
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) < b.gcm.NonceSize()+b.gcm.Overhead() {
		return flow, errors.New("invalid browser flow")
	}
	size := b.gcm.NonceSize()
	plain, err := b.gcm.Open(nil, data[:size], data[size:], []byte("go-auth/browser-flow/v1"))
	if err != nil {
		return flow, errors.New("invalid browser flow")
	}
	if err := json.Unmarshal(plain, &flow); err != nil || flow.ExpiresAt <= time.Now().Unix() || flow.ExpiresAt > time.Now().Add(flowTTL).Unix()+1 || flow.State == "" || flow.Verifier == "" {
		return browserFlow{}, errors.New("invalid browser flow")
	}
	return flow, nil
}

func (app *Application) browserLogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !app.Browser.validCSRF(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "csrf_failed"})
		return
	}
	if c.Param("provider") != app.Browser.config.Provider {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	state, err := randomBrowserValue()
	if err != nil {
		app.browserUnavailable(c)
		return
	}
	verifier, err := randomBrowserValue()
	if err != nil {
		app.browserUnavailable(c)
		return
	}
	origin := c.GetHeader("Origin")
	if origin == "" || !app.Browser.matchesOrigin(origin) {
		origin = app.Browser.config.Origin
	}
	challenge := sha256.Sum256([]byte(verifier))
	output, err := app.Handler.Login(c.Request.Context(), &handler.LoginInput{
		Provider: app.Browser.config.Provider, ClientID: app.Browser.config.ClientID,
		RedirectURI: origin + "/auth/callback", State: state,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(challenge[:]), CodeChallengeMethod: "S256",
	})
	if err != nil {
		if isInvalidLoginRequest(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		app.browserUnavailable(c)
		return
	}
	cookie, err := app.Browser.encodeFlow(browserFlow{State: state, Verifier: verifier, Origin: origin, ExpiresAt: time.Now().Add(flowTTL).Unix()})
	if err != nil {
		app.browserUnavailable(c)
		return
	}
	app.Browser.setCookie(c, flowCookie, cookie, int(flowTTL/time.Second))
	c.JSON(http.StatusOK, gin.H{"redirect_url": output.RedirectURL})
}

func (app *Application) browserCallback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	cookie, err := c.Cookie(flowCookie)
	app.Browser.setCookie(c, flowCookie, "", -1)
	if err != nil {
		app.browserLoginError(c, "", "invalid_request")
		return
	}
	flow, err := app.Browser.decodeFlow(cookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(flow.State), []byte(c.Query("state"))) != 1 {
		app.browserLoginError(c, flow.Origin, "invalid_request")
		return
	}
	if category := c.Query("error"); category != "" {
		switch category {
		case "access_denied", "expired_request", "temporarily_unavailable":
			app.browserLoginError(c, flow.Origin, category)
		default:
			app.browserLoginError(c, flow.Origin, "invalid_request")
		}
		return
	}
	origin := flow.Origin
	if origin == "" || !app.Browser.matchesOrigin(origin) {
		origin = app.Browser.config.Origin
	}
	output, err := app.Handler.Exchange(c.Request.Context(), &handler.ExchangeInput{
		ClientCredentials: app.Browser.credentials(), Provider: app.Browser.config.Provider,
		Code: c.Query("code"), CodeVerifier: flow.Verifier, RedirectURI: origin + "/auth/callback",
	})
	if err != nil {
		category := "temporarily_unavailable"
		if errors.Is(err, handler.ErrInvalidGrant) || errors.Is(err, handler.ErrInvalidExchangeRequest) {
			category = "invalid_request"
		}
		app.browserLoginError(c, origin, category)
		return
	}
	app.Browser.setSession(c, output.AccessToken, output.RefreshToken, output.ExpiresIn, output.RefreshExpiresIn)
	c.Redirect(http.StatusFound, origin+"/app")
}

func (app *Application) browserLoginError(c *gin.Context, origin, category string) {
	if origin == "" || !app.Browser.matchesOrigin(origin) {
		origin = app.Browser.config.Origin
	}
	c.Redirect(http.StatusFound, origin+"/auth/login?error="+url.QueryEscape(category))
}

func (app *Application) browserUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
}

func (app *Application) browserRefresh(c *gin.Context) {
	token, _ := c.Cookie(refreshCookie)
	output, err := app.Handler.Refresh(c.Request.Context(), &handler.RefreshInput{
		ClientCredentials: app.Browser.credentials(), Provider: app.Browser.config.Provider, RefreshToken: token,
	})
	if err != nil {
		if errors.Is(err, handler.ErrInvalidGrant) {
			app.Browser.clearSession(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		app.browserUnavailable(c)
		return
	}
	app.Browser.setSession(c, output.AccessToken, output.RefreshToken, output.ExpiresIn, output.RefreshExpiresIn)
	c.Status(http.StatusNoContent)
}

// Logout first revokes a valid access session. If the access cookie has expired,
// a live refresh token proves ownership: rotate it and revoke its resulting session.
func (app *Application) browserLogout(c *gin.Context) {
	input := handler.LogoutInput{ClientCredentials: app.Browser.credentials(), Provider: app.Browser.config.Provider,
		AuthorizationHeader: c.GetHeader("Authorization")}
	if err := app.Handler.Logout(c.Request.Context(), &input); err != nil {
		app.browserUnavailable(c)
		return
	}
	refresh, _ := c.Cookie(refreshCookie)
	if refresh != "" {
		output, err := app.Handler.Refresh(c.Request.Context(), &handler.RefreshInput{
			ClientCredentials: app.Browser.credentials(), Provider: app.Browser.config.Provider, RefreshToken: refresh,
		})
		if err != nil && !errors.Is(err, handler.ErrInvalidGrant) {
			app.browserUnavailable(c)
			return
		}
		if err == nil {
			input.AuthorizationHeader = "Bearer " + output.AccessToken
			if err := app.Handler.Logout(c.Request.Context(), &input); err != nil {
				app.browserUnavailable(c)
				return
			}
		}
	}
	app.Browser.clearSession(c)
	c.Status(http.StatusNoContent)
}
