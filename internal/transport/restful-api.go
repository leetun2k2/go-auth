package transport

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-auth/internal/middleware"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

// extractBearerClientCredentials extracts client credentials for endpoints
// that also carry a bearer session token in the same Authorization header
// (me, updateProfile, logout). Basic auth is rejected here: it would consume
// the Authorization header for client credentials, leaving no room for the
// bearer token, so callers must use the X-Client-Id/X-Client-Secret header
// pair alongside Authorization: Bearer <token> instead.
func extractBearerClientCredentials(r *http.Request) (credential.Credentials, error) {
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "basic ") {
		return credential.Credentials{}, credential.ErrInvalidClient
	}
	return credential.Extract(r)
}

func (app *Application) RegisterRestfulApi(engine *gin.Engine) error {
	engine.Use(middleware.CorrelationID)

	engine.GET("/health", app.health)
	engine.GET("/ready", app.ready)

	v1 := engine.Group("/api/auth/v1")
	{
		v1.GET("/login/:provider", app.login)
		v1.GET("/callback/:provider", app.callback)
		v1.POST("/exchange", app.exchange)
		v1.GET("/me", app.me)
		v1.PATCH("/profile", app.updateProfile)
		v1.POST("/logout", app.logout)
	}

	admin := engine.Group("/api/admin/v1")
	{
		admin.PUT("/clients/:provider/:client_id", app.upsertClient)
	}
	return nil
}

func (app *Application) health(c *gin.Context) {
	ctx := c.Request.Context()
	input := handler.HealthInput{}
	output, err := app.Handler.Health(ctx, &input)
	if err != nil {
		c.JSON(500, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(output.StatusCode, output)
}

func (app *Application) ready(c *gin.Context) {
	ctx := c.Request.Context()
	input := handler.ReadyInput{}
	output, err := app.Handler.Ready(ctx, &input)
	if err != nil {
		c.JSON(500, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(output.StatusCode, output)
}

func (app *Application) login(c *gin.Context) {
	ctx := c.Request.Context()
	input := handler.LoginInput{
		Provider:            c.Param("provider"),
		ClientID:            c.Query("client_id"),
		RedirectURI:         c.Query("redirect_uri"),
		State:               c.Query("state"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
	}

	output, err := app.Handler.Login(ctx, &input)
	if err != nil {
		if isInvalidLoginRequest(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		app.Logger.Error(ctx, "login start failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "temporarily_unavailable"})
		return
	}

	c.Redirect(http.StatusFound, output.RedirectURL)
}

func (app *Application) callback(c *gin.Context) {
	ctx := c.Request.Context()
	input := handler.CallbackInput{
		Provider: c.Param("provider"),
		State:    c.Query("state"),
		Code:     c.Query("code"),
		Error:    c.Query("error"),
	}

	output, err := app.Handler.Callback(ctx, &input)
	if err != nil {
		if errors.Is(err, handler.ErrInvalidCallbackRequest) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		app.Logger.Error(ctx, "provider callback failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "temporarily_unavailable"})
		return
	}

	c.Redirect(http.StatusFound, output.RedirectURL)
}

// exchangeRequest is the JSON or form payload for POST /api/auth/v1/exchange.
// gin.Context.ShouldBind selects the JSON or form binder from the request
// Content-Type, matching the spec's "JSON or form data" requirement.
type exchangeRequest struct {
	Code         string `json:"code" form:"code"`
	CodeVerifier string `json:"code_verifier" form:"code_verifier"`
	RedirectURI  string `json:"redirect_uri" form:"redirect_uri"`
}

func (app *Application) exchange(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "no-store")

	creds, err := credential.Extract(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}

	var req exchangeRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		return
	}

	input := handler.ExchangeInput{
		ClientCredentials: creds,
		Provider:          c.GetHeader("X-Client-Provider"),
		Code:              req.Code,
		CodeVerifier:      req.CodeVerifier,
		RedirectURI:       req.RedirectURI,
	}

	output, err := app.Handler.Exchange(ctx, &input)
	if err != nil {
		switch {
		case errors.Is(err, handler.ErrInvalidClient):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		case errors.Is(err, handler.ErrInvalidExchangeRequest), errors.Is(err, handler.ErrInvalidGrant):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		default:
			app.Logger.Error(ctx, "code exchange failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "temporarily_unavailable"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": output.AccessToken,
		"token_type":   output.TokenType,
		"expires_in":   output.ExpiresIn,
		"user": gin.H{
			"id":           output.User.ID,
			"email":        output.User.Email,
			"display_name": output.User.DisplayName,
			"avatar_url":   output.User.AvatarURL,
		},
	})
}

func (app *Application) me(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "no-store")

	creds, err := extractBearerClientCredentials(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	input := handler.MeInput{
		ClientCredentials:   creds,
		Provider:            c.GetHeader("X-Client-Provider"),
		AuthorizationHeader: c.GetHeader("Authorization"),
	}

	output, err := app.Handler.Me(ctx, &input)
	if err != nil {
		if errors.Is(err, handler.ErrUnauthenticated) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		app.Logger.Error(ctx, "current user lookup failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":      output.UserID,
		"identity_id":  output.IdentityID,
		"email":        nullableString(output.Email),
		"display_name": output.DisplayName,
		"avatar_url":   output.AvatarURL,
	})
}

type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

func (app *Application) updateProfile(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "no-store")

	creds, err := extractBearerClientCredentials(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	input := handler.UpdateProfileInput{
		ClientCredentials:   creds,
		Provider:            c.GetHeader("X-Client-Provider"),
		AuthorizationHeader: c.GetHeader("Authorization"),
		DisplayName:         req.DisplayName,
		AvatarURL:           req.AvatarURL,
	}

	output, err := app.Handler.UpdateProfile(ctx, &input)
	if err != nil {
		switch {
		case errors.Is(err, handler.ErrUnauthenticated):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		case errors.Is(err, handler.ErrInvalidProfileRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		default:
			app.Logger.Error(ctx, "update profile failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "temporarily_unavailable"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":      output.UserID,
		"email":        nullableString(output.Email),
		"display_name": output.DisplayName,
		"avatar_url":   output.AvatarURL,
	})
}

func (app *Application) logout(c *gin.Context) {
	ctx := c.Request.Context()
	c.Header("Cache-Control", "no-store")

	creds, err := extractBearerClientCredentials(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}

	input := handler.LogoutInput{
		ClientCredentials:   creds,
		Provider:            c.GetHeader("X-Client-Provider"),
		AuthorizationHeader: c.GetHeader("Authorization"),
	}

	if err := app.Handler.Logout(ctx, &input); err != nil {
		if errors.Is(err, handler.ErrInvalidClient) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
			return
		}
		app.Logger.Error(ctx, "logout failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporarily_unavailable"})
		return
	}

	c.Status(http.StatusNoContent)
}

type upsertClientRequest struct {
	Name         string   `json:"name"`
	ClientSecret string   `json:"client_secret"`
	AuthSecret   string   `json:"auth_secret"`
	RedirectURL  string   `json:"redirect_url"`
	RedirectURIs []string `json:"redirect_uris"`
	Enabled      *bool    `json:"enabled"`
}

func (app *Application) upsertClient(c *gin.Context) {
	ctx := c.Request.Context()

	var req upsertClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	input := handler.UpsertClientInput{
		AuthorizationHeader: c.GetHeader("Authorization"),
		Provider:            c.Param("provider"),
		ClientID:            c.Param("client_id"),
		Name:                req.Name,
		ClientSecret:        req.ClientSecret,
		AuthSecret:          req.AuthSecret,
		RedirectURL:         req.RedirectURL,
		RedirectURIs:        req.RedirectURIs,
		Enabled:             req.Enabled,
	}

	if err := app.Handler.UpsertClient(ctx, &input); err != nil {
		switch {
		case errors.Is(err, handler.ErrUnauthenticated):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		case errors.Is(err, handler.ErrUnsupportedProvider), errors.Is(err, handler.ErrInvalidAdminRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		default:
			app.Logger.Error(ctx, "upsert client failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "temporarily_unavailable"})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isInvalidLoginRequest(err error) bool {
	switch {
	case errors.Is(err, handler.ErrUnsupportedProvider),
		errors.Is(err, handler.ErrUnknownClient),
		errors.Is(err, handler.ErrInvalidRedirectURI),
		errors.Is(err, handler.ErrInvalidLoginRequest):
		return true
	default:
		return false
	}
}
