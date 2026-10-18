package iam

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/leetun2k2/go-bedrock/logx"
)

type AuthenticatedUser struct {
	UserID    uuid.UUID
	Email     string
	SessionID uuid.UUID
}

type AuthenticateFunc func(ctx context.Context, authHeader, clientID, clientSecret, provider string) (*AuthenticatedUser, error)

type Handler struct {
	repo         *Repository
	logger       *logx.Logger
	authenticate AuthenticateFunc
}

func NewHandler(repo *Repository, logger *logx.Logger, authFunc AuthenticateFunc) *Handler {
	return &Handler{
		repo:         repo,
		logger:       logger,
		authenticate: authFunc,
	}
}

func (h *Handler) authMiddleware(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	clientID := c.GetHeader("X-Client-Id")
	clientSecret := c.GetHeader("X-Client-Secret")
	provider := c.GetHeader("X-Client-Provider")
	if provider == "" {
		provider = "google"
	}

	authUser, err := h.authenticate(c.Request.Context(), authHeader, clientID, clientSecret, provider)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorEnvelope{
			Error: ErrorDetail{Code: "unauthorized", Message: "Authentication required"},
		})
		return
	}

	access, err := h.repo.GetUserAccess(c.Request.Context(), authUser.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorEnvelope{
				Error: ErrorDetail{Code: "unauthorized", Message: "Authentication required"},
			})
			return
		}
		h.logger.Error(c.Request.Context(), "load user access failed", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorEnvelope{
			Error: ErrorDetail{Code: "internal", Message: "Internal server error"},
		})
		return
	}

	c.Set("user", authUser)
	c.Set("access", access)
	c.Next()
}

// AuthMiddleware exposes authMiddleware for routes registered outside
// RegisterRoutes that still need the same session verification.
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return h.authMiddleware
}

// RequireAdmin exposes requireAdmin for routes registered outside
// RegisterRoutes that still need the same admin gate.
func (h *Handler) RequireAdmin() gin.HandlerFunc {
	return h.requireAdmin
}

func (h *Handler) requireAdmin(c *gin.Context) {
	val, exists := c.Get("access")
	if !exists {
		c.AbortWithStatusJSON(http.StatusForbidden, ErrorEnvelope{
			Error: ErrorDetail{Code: "forbidden", Message: "Admin role required"},
		})
		return
	}
	access := val.(*UserAccess)
	if !access.IsAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, ErrorEnvelope{
			Error: ErrorDetail{Code: "forbidden", Message: "Admin role required"},
		})
		return
	}
	c.Next()
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("")
	g.Use(h.authMiddleware)

	// Access info: any valid session, admins and non-admins alike.
	g.GET("/access", h.getAccess)

	// Users
	g.GET("/users", h.requireAdmin, h.listUsers)
	g.GET("/users/:id", h.requireAdmin, h.getUser)
	g.PUT("/users/:id/roles", h.requireAdmin, h.updateUserRoles)

	// Identities
	g.GET("/identities", h.requireAdmin, h.listIdentities)

	// Roles
	g.GET("/roles", h.requireAdmin, h.listRoles)
	g.GET("/roles/:id", h.requireAdmin, h.getRole)
	g.POST("/roles", h.requireAdmin, h.createRole)
	g.PATCH("/roles/:id", h.requireAdmin, h.updateRole)
	g.DELETE("/roles/:id", h.requireAdmin, h.deleteRole)

	// Sessions
	g.GET("/sessions", h.requireAdmin, h.listSessions)
	g.DELETE("/sessions/:id", h.requireAdmin, h.revokeSession)

	// Clients
	g.GET("/clients", h.requireAdmin, h.listClients)
	g.GET("/clients/:id", h.requireAdmin, h.getClient)
	g.PATCH("/clients/:id", h.requireAdmin, h.updateClientEnabled)
}

// parsePagination reads page and page_size query params with safe defaults.
// Invalid or negative values fall back to defaults; page_size is clamped to 100.
func parsePagination(c *gin.Context) (int, int) {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	page, err := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	if err != nil || page < 1 {
		page = defaultPage
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if err != nil || pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return page, pageSize
}

func totalPages(total int64, pageSize int) int {
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

// parseIDParam parses the :id route param as a UUID. On failure it writes the
// 400 response and returns false so the caller can abort.
func (h *Handler) parseIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{
			Error: ErrorDetail{Code: "invalid_request", Message: "Invalid ID format"},
		})
		return uuid.UUID{}, false
	}
	return id, true
}

// handleError maps a repository error to the appropriate HTTP response.
func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrLastAdmin):
		c.JSON(http.StatusConflict, ErrorEnvelope{
			Error: ErrorDetail{Code: "last_admin_conflict", Message: "Cannot remove the last administrator"},
		})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, ErrorEnvelope{
			Error: ErrorDetail{Code: "not_found", Message: "Resource not found"},
		})
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, ErrorEnvelope{
			Error: ErrorDetail{Code: "conflict", Message: "Resource conflict"},
		})
	default:
		h.logger.Error(c.Request.Context(), "iam handler error", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorEnvelope{
			Error: ErrorDetail{Code: "internal", Message: "Internal server error"},
		})
	}
}

func (h *Handler) getAccess(c *gin.Context) {
	val, _ := c.Get("access")
	access := val.(*UserAccess)
	c.JSON(http.StatusOK, ResponseEnvelope[*UserAccess]{Data: access})
}

func (h *Handler) listUsers(c *gin.Context) {
	page, pageSize := parsePagination(c)
	search := c.Query("search")
	role := c.Query("role")

	users, total, err := h.repo.ListUsers(c.Request.Context(), page, pageSize, search, role)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ResponseEnvelope[[]UserSummary]{
		Data: users,
		Pagination: &Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages(total, pageSize),
		},
	})
}

func (h *Handler) getUser(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	detail, err := h.repo.GetUser(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[*UserDetail]{Data: detail})
}

func (h *Handler) updateUserRoles(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	var req UpdateUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{Error: ErrorDetail{Code: "invalid_request", Message: err.Error()}})
		return
	}

	if err := h.repo.UpdateUserRoles(c.Request.Context(), id, req.RoleIDs); err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[string]{Data: "User roles updated successfully"})
}

func (h *Handler) listIdentities(c *gin.Context) {
	page, pageSize := parsePagination(c)
	provider := c.Query("provider")

	list, total, err := h.repo.ListIdentities(c.Request.Context(), page, pageSize, provider)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ResponseEnvelope[[]IdentitySummary]{
		Data: list,
		Pagination: &Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages(total, pageSize),
		},
	})
}

func (h *Handler) listRoles(c *gin.Context) {
	page, pageSize := parsePagination(c)

	list, total, err := h.repo.ListRoles(c.Request.Context(), page, pageSize)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ResponseEnvelope[[]RoleItem]{
		Data: list,
		Pagination: &Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages(total, pageSize),
		},
	})
}

func (h *Handler) getRole(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	role, err := h.repo.GetRole(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[*RoleItem]{Data: role})
}

func (h *Handler) createRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{Error: ErrorDetail{Code: "invalid_request", Message: err.Error()}})
		return
	}

	role, err := h.repo.CreateRole(c.Request.Context(), req.Key, req.Name)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ResponseEnvelope[*RoleItem]{Data: role})
}

func (h *Handler) updateRole(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{Error: ErrorDetail{Code: "invalid_request", Message: err.Error()}})
		return
	}

	role, err := h.repo.UpdateRole(c.Request.Context(), id, req.Name)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[*RoleItem]{Data: role})
}

func (h *Handler) deleteRole(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	if err := h.repo.DeleteRole(c.Request.Context(), id); err != nil {
		h.handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listSessions(c *gin.Context) {
	page, pageSize := parsePagination(c)

	var userID *uuid.UUID
	if raw := c.Query("user_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorEnvelope{
				Error: ErrorDetail{Code: "invalid_request", Message: "Invalid user_id format"},
			})
			return
		}
		userID = &parsed
	}

	activeOnly, err := parseBoolQuery(c, "active")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{
			Error: ErrorDetail{Code: "invalid_request", Message: "Invalid active value"},
		})
		return
	}

	list, total, err := h.repo.ListSessions(c.Request.Context(), page, pageSize, userID, activeOnly)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ResponseEnvelope[[]SessionItem]{
		Data: list,
		Pagination: &Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages(total, pageSize),
		},
	})
}

// parseBoolQuery parses an optional boolean query param. An absent param
// returns false with no error.
func parseBoolQuery(c *gin.Context, name string) (bool, error) {
	raw := c.Query(name)
	if raw == "" {
		return false, nil
	}
	return strconv.ParseBool(raw)
}

func (h *Handler) revokeSession(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	if err := h.repo.RevokeSession(c.Request.Context(), id); err != nil {
		h.handleError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) listClients(c *gin.Context) {
	page, pageSize := parsePagination(c)

	list, total, err := h.repo.ListClients(c.Request.Context(), page, pageSize)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, ResponseEnvelope[[]ClientRegistrationItem]{
		Data: list,
		Pagination: &Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: total,
			TotalPages: totalPages(total, pageSize),
		},
	})
}

func (h *Handler) getClient(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	item, err := h.repo.GetClient(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[*ClientRegistrationItem]{Data: item})
}

func (h *Handler) updateClientEnabled(c *gin.Context) {
	id, ok := h.parseIDParam(c)
	if !ok {
		return
	}

	var req UpdateClientEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorEnvelope{Error: ErrorDetail{Code: "invalid_request", Message: err.Error()}})
		return
	}

	if err := h.repo.UpdateClientEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, ResponseEnvelope[string]{Data: "Client updated successfully"})
}
