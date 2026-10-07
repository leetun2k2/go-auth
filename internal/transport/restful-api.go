package transport

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/leetun2k2/go-auth/internal/handler"
	"github.com/leetun2k2/go-auth/internal/middleware"
	"github.com/leetun2k2/go-bedrock/authx/credential"
)

func (app *Application) RegisterRestfulApi(engine *gin.Engine) error {
	engine.Use(middleware.CorrelationID)

	engine.GET("/health", app.health)
	engine.GET("/ready", app.ready)
	engine.GET("/auth/login/:provider", app.login)
	engine.GET("/auth/callback/:provider", app.callback)
	engine.POST("/auth/exchange", app.exchange)
	engine.GET("/auth/me", app.me)
	engine.PATCH("/auth/profile", app.updateProfile)
	engine.POST("/auth/logout", app.logout)
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

// exchangeRequest is the JSON or form payload for POST /auth/exchange.
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

	input := handler.MeInput{AuthorizationHeader: c.GetHeader("Authorization")}

	output, err := app.Handler.Me(ctx, &input)
	if err != nil {
		if !errors.Is(err, handler.ErrUnauthenticated) {
			app.Logger.Error(ctx, "current user lookup failed", "error", err)
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
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

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if _, ok := credential.ExtractBearerToken(c.GetHeader("Authorization")); !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	input := handler.UpdateProfileInput{
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

	creds, err := credential.Extract(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}

	input := handler.LogoutInput{
		ClientCredentials:   creds,
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
