package httpapi

import (
	"html/template"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/yourGaomu/short_links/back/internal/modules/links"
)

var publicPageTemplate = template.Must(template.New("public-page").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}}</title>
<style>body{font-family:system-ui,sans-serif;background:#f4f6f5;color:#17231f;margin:0;min-height:100vh;display:grid;place-items:center}.panel{width:min(440px,calc(100% - 40px));background:#fff;border:1px solid #dce4df;border-radius:8px;padding:28px;box-sizing:border-box}h1{font-size:20px;margin:0 0 12px}p{line-height:1.6;color:#52615a}label{display:block;font-size:14px;margin:20px 0 8px}input{width:100%;box-sizing:border-box;padding:12px;border:1px solid #bdc9c2;border-radius:5px;font:inherit}button{margin-top:14px;border:0;border-radius:5px;padding:11px 16px;background:#126a50;color:#fff;font:inherit;cursor:pointer}</style></head>
<body><main class="panel"><h1>{{.Title}}</h1><p>{{.Message}}</p>{{if .Challenge}}<form method="post" action="{{.Action}}"><label for="password">访问口令</label><input id="password" name="password" type="password" autocomplete="current-password" maxlength="1024" required autofocus><button type="submit">继续访问</button></form>{{end}}</main></body></html>`))

func registerLinkRoutes(router *gin.Engine, deps Dependencies) {
	// 1. 管理 API 统一认证；每个业务路由在 Handler 前执行对应功能权限检查。
	service := deps.Links
	api := router.Group("/api/v1")
	api.Use(authenticate(deps.Auth))
	api.POST("/links", requireCreateLinkPermission(deps.DB), func(c *gin.Context) {
		var input links.CreateRequest
		if !decodeJSON(c, &input) {
			return
		}
		result, err := service.Create(c.Request.Context(), linkActor(c), input, linkMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.GET("/links", requirePermission(deps.DB, "links.read"), func(c *gin.Context) {
		page, pageSize, ok := parsePagination(c)
		if !ok {
			return
		}
		filters := map[string]string{}
		for _, key := range []string{"workspaceId", "status", "availability", "targetType", "code", "createdFrom", "createdTo"} {
			filters[key] = c.Query(key)
		}
		result, err := service.List(c.Request.Context(), linkActor(c), page, pageSize, filters)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/links/:linkId", requirePermission(deps.DB, "links.read"), func(c *gin.Context) {
		result, err := service.Get(c.Request.Context(), linkActor(c), c.Param("linkId"))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/links/:linkId/renew", requirePermission(deps.DB, "links.manage"), func(c *gin.Context) {
		var input links.RenewRequest
		if !decodeJSON(c, &input) {
			return
		}
		result, err := service.Renew(c.Request.Context(), linkActor(c), c.Param("linkId"), input, linkMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/links/:linkId/rotate-code", requirePermission(deps.DB, "links.manage"), func(c *gin.Context) {
		var input links.RotateRequest
		if !decodeJSON(c, &input) {
			return
		}
		result, err := service.Rotate(c.Request.Context(), linkActor(c), c.Param("linkId"), input, linkMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/links/:linkId/reset-visits", requirePermission(deps.DB, "links.reset_visits"), func(c *gin.Context) {
		result, err := service.ResetVisits(c.Request.Context(), linkActor(c), c.Param("linkId"), linkMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/links/:linkId/disable", requirePermission(deps.DB, "links.manage"), linkStatusHandler(service, "disabled"))
	api.POST("/links/:linkId/enable", requirePermission(deps.DB, "links.manage"), linkStatusHandler(service, "enabled"))

	// 2. 公开解析入口不要求用户 Token；HTML 页面与 JSON 管理接口严格分离。
	router.GET("/r/:code", func(c *gin.Context) { publicLinkHandler(c, service, false) })
	router.POST("/r/:code/password", func(c *gin.Context) { publicLinkHandler(c, service, true) })
}

func linkStatusHandler(service *links.Service, status string) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := service.SetStatus(c.Request.Context(), linkActor(c), c.Param("linkId"), status, linkMeta(c))
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func linkActor(c *gin.Context) links.Actor {
	identity := currentAuth(c)
	workspaceID, _ := parsePublicID("ws", identity.User.DefaultWorkspaceID)
	return links.Actor{UserID: identity.User.ID, WorkspaceID: workspaceID, Role: identity.User.Role}
}

func linkMeta(c *gin.Context) links.RequestMeta {
	return links.RequestMeta{SourceIP: c.ClientIP(), UserAgent: truncateLogValue(c.Request.UserAgent(), 1024), RequestID: requestID(c)}
}

func publicLinkHandler(c *gin.Context, service *links.Service, submit bool) {
	// 1. 访问入口只接受服务端解析的短码与来源信息，不读任何目标地址参数。
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	password := ""
	if submit {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		if err := c.Request.ParseForm(); err != nil {
			writePublicPage(c, links.PublicResult{StatusCode: http.StatusBadRequest, PageTitle: "请求无效", PageText: "请重新打开短链页面后重试。"}, c.Param("code"))
			return
		}
		password = c.Request.PostForm.Get("password")
		if len(password) > 1024 {
			writePublicPage(c, links.PublicResult{StatusCode: http.StatusBadRequest, PageTitle: "请求无效", PageText: "访问口令格式无效。"}, c.Param("code"))
			return
		}
	}
	result, err := service.Access(c.Request.Context(), c.Param("code"), linkMeta(c), password, submit)
	if err != nil {
		writePublicPage(c, links.PublicResult{StatusCode: http.StatusServiceUnavailable, PageTitle: "服务暂不可用", PageText: "短链服务暂时不可用，请稍后重试。"}, c.Param("code"))
		return
	}
	writePublicPage(c, result, c.Param("code"))
}

func writePublicPage(c *gin.Context, result links.PublicResult, code string) {
	if result.StatusCode == http.StatusFound && result.Location != "" {
		c.Redirect(http.StatusFound, result.Location)
		return
	}
	action := "/r/" + url.PathEscape(code) + "/password"
	c.Status(result.StatusCode)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = publicPageTemplate.Execute(c.Writer, struct {
		Title, Message, Action string
		Challenge              bool
	}{result.PageTitle, result.PageText, action, result.Challenge})
}
