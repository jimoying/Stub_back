package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yourGaomu/short_links/back/internal/modules/auth"
)

func registerAdminRoutes(admin *gin.RouterGroup, service *auth.Service) {
	// 1. 管理 API 在 RequireAuth 和 RequireAdmin 后执行；服务层仍校验资源状态。
	admin.GET("/settings/registration", func(c *gin.Context) {
		result, err := service.GetRegistrationSettings(c.Request.Context())
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.PATCH("/settings/registration", func(c *gin.Context) {
		var input auth.RegistrationSettingsPatch
		if !decodeJSON(c, &input) {
			return
		}
		result, err := service.UpdateRegistrationSettings(c.Request.Context(), currentAuth(c).User.ID, input, authMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.GET("/users", func(c *gin.Context) {
		page, pageSize, ok := parsePagination(c)
		if !ok {
			return
		}
		result, err := service.AdminUsers(c.Request.Context(), c.Query("query"), c.Query("role"), page, pageSize)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if !recordAdminRead(c, service, "admin.users.read", "user_collection", "all", map[string]any{
			"query": c.Query("query"), "roleFilter": c.Query("role"), "page": page, "pageSize": pageSize,
		}) {
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.GET("/users/:userId", func(c *gin.Context) {
		id, err := parsePublicID("usr", c.Param("userId"))
		if err != nil {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		result, err := service.AdminUser(c.Request.Context(), id)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if !recordAdminRead(c, service, "admin.user.read", "user", id.String(), nil) {
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.GET("/users/:userId/quotas", func(c *gin.Context) {
		id, err := parsePublicID("usr", c.Param("userId"))
		if err != nil {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		result, err := service.UserQuotas(c.Request.Context(), id)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if !recordAdminRead(c, service, "admin.user_quotas.read", "user", id.String(), nil) {
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.PATCH("/users/:userId/quotas", func(c *gin.Context) {
		id, err := parsePublicID("usr", c.Param("userId"))
		if err != nil {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		var input auth.QuotaPatch
		if !decodeJSON(c, &input) {
			return
		}
		result, err := service.UpdateUserQuotas(c.Request.Context(), currentAuth(c).User.ID, id, input, authMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.GET("/permissions/catalog", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"items": service.PermissionCatalog()}) })
	admin.GET("/users/:userId/permissions", func(c *gin.Context) {
		id, err := parsePublicID("usr", c.Param("userId"))
		if err != nil {
			writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		result, err := service.AdminUserPermissions(c.Request.Context(), id)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		if !recordAdminRead(c, service, "admin.user_permissions.read", "user", id.String(), nil) {
			return
		}
		c.JSON(http.StatusOK, result)
	})
	admin.PUT("/users/:userId/permissions/:permissionCode", func(c *gin.Context) { updatePermission(c, service, true) })
	admin.DELETE("/users/:userId/permissions/:permissionCode", func(c *gin.Context) { updatePermission(c, service, false) })
	admin.POST("/invites/revoke-unused", func(c *gin.Context) {
		var input struct {
			UserID     string `json:"userId,omitempty"`
			ConfirmAll bool   `json:"confirmAll"`
		}
		if !decodeJSON(c, &input) {
			return
		}
		var userID *uuid.UUID
		if input.UserID != "" {
			id, err := parsePublicID("usr", input.UserID)
			if err != nil {
				writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
				return
			}
			userID = &id
		}
		result, err := service.RevokeUnusedInvites(c.Request.Context(), currentAuth(c).User.ID, userID, input.ConfirmAll, authMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func recordAdminRead(c *gin.Context, service *auth.Service, event, resourceType, resourceID string, details map[string]any) bool {
	identity := currentAuth(c)
	if err := service.RecordAdminRead(c.Request.Context(), identity.User.ID, event, resourceType, resourceID, authMeta(c), details); err != nil {
		writeServiceError(c, err)
		return false
	}
	return true
}

func updatePermission(c *gin.Context, service *auth.Service, grant bool) {
	// 1. 授权目标只能由路径用户 ID 确定，响应立即反映最新权限。
	id, err := parsePublicID("usr", c.Param("userId"))
	if err != nil {
		writeError(c, http.StatusNotFound, "user_not_found", "用户不存在")
		return
	}
	result, err := service.SetUserPermission(c.Request.Context(), currentAuth(c).User.ID, id, c.Param("permissionCode"), grant, authMeta(c))
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
