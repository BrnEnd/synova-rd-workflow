package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"synova-rd-workflow/internal/domain"
	adminSvc "synova-rd-workflow/internal/service/admin"
)

type Handler struct {
	auth         *adminSvc.AuthService
	resources    *adminSvc.ResourceService
	rd           adminSvc.RDStationClient
	cookieSecure bool
}

func New(auth *adminSvc.AuthService, resources *adminSvc.ResourceService, rd adminSvc.RDStationClient, cookieSecure bool) *Handler {
	return &Handler{auth: auth, resources: resources, rd: rd, cookieSecure: cookieSecure}
}

func (h *Handler) RegisterPublicRoutes(r gin.IRouter, loginMiddleware gin.HandlerFunc) {
	r.POST("/login", loginMiddleware, h.login)
	r.POST("/logout", h.logout)
}

func (h *Handler) RegisterProtectedRoutes(r gin.IRouter) {
	r.POST("/change-password", h.changePassword)
	r.GET("/me", h.me)
	r.GET("/collaborators", h.listCollaborators)
	r.POST("/collaborators", h.upsertCollaborator)
	r.PUT("/collaborators/:id", h.upsertCollaborator)
	r.GET("/alerts", h.listAlerts)
	r.POST("/alerts", h.upsertAlert)
	r.PUT("/alerts/:id", h.upsertAlert)
	r.POST("/alerts/:id/test", h.testAlert)
	r.GET("/allowlist", h.listAllowlist)
	r.POST("/allowlist", h.upsertAllowlist)
	r.PUT("/allowlist/:id", h.upsertAllowlist)
	r.GET("/rd-station/stages", h.listStages)
}

func (h *Handler) login(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	token, cfg, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, adminSvc.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	h.setCookie(c, token, 28800)
	c.JSON(http.StatusOK, gin.H{"must_change_password": cfg.MustChangePassword, "email": cfg.Email})
}

func (h *Handler) logout(c *gin.Context) {
	h.setCookie(c, "", -1)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) changePassword(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	token, cfg, err := h.auth.ChangePassword(c.Request.Context(), req.CurrentPassword, req.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password"})
		case errors.Is(err, adminSvc.ErrCurrentPassword):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_current_password"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}
		return
	}
	h.setCookie(c, token, 28800)
	c.JSON(http.StatusOK, gin.H{"ok": true, "email": cfg.Email, "must_change_password": false})
}

func (h *Handler) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"email":                c.GetString("admin_email"),
		"must_change_password": mustChange(c),
	})
}

func (h *Handler) listCollaborators(c *gin.Context) {
	items, err := h.resources.ListCollaborators(c.Request.Context(), c.Query("active") == "true")
	respondList(c, items, err)
}

func (h *Handler) upsertCollaborator(c *gin.Context) {
	var req domain.Collaborator
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if id := c.Param("id"); id != "" {
		req.ID = id
	}
	item, err := h.resources.UpsertCollaborator(c.Request.Context(), req)
	respondMutation(c, item, err)
}

func (h *Handler) listAlerts(c *gin.Context) {
	items, err := h.resources.ListAlerts(c.Request.Context(), c.Query("active") == "true")
	respondList(c, items, err)
}

func (h *Handler) upsertAlert(c *gin.Context) {
	var req domain.Alert
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if id := c.Param("id"); id != "" {
		req.ID = id
	}
	item, err := h.resources.UpsertAlert(c.Request.Context(), req)
	respondMutation(c, item, err)
}

func (h *Handler) testAlert(c *gin.Context) {
	result, err := h.resources.RunAlertNow(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondMutation(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": result})
}

func (h *Handler) listAllowlist(c *gin.Context) {
	items, err := h.resources.ListAllowlist(c.Request.Context(), c.Query("active") == "true")
	respondList(c, items, err)
}

func (h *Handler) upsertAllowlist(c *gin.Context) {
	var req domain.AllowlistEntry
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}
	if id := c.Param("id"); id != "" {
		req.ID = id
	}
	item, err := h.resources.UpsertAllowlist(c.Request.Context(), req)
	respondMutation(c, item, err)
}

func (h *Handler) listStages(c *gin.Context) {
	stages, err := h.rd.GetDealStages(c.Request.Context())
	respondList(c, stages, err)
}

func (h *Handler) setCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func respondList(c *gin.Context, items interface{}, err error) {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func respondMutation(c *gin.Context, item interface{}, err error) {
	if err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_input"})
		case errors.Is(err, adminSvc.ErrDuplicate):
			c.JSON(http.StatusConflict, gin.H{"error": "already_exists"})
		case errors.Is(err, adminSvc.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}
		return
	}
	c.JSON(http.StatusOK, item)
}

func mustChange(c *gin.Context) bool {
	v, exists := c.Get("must_change_password")
	if !exists {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
