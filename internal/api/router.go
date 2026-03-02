package api

import (
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
)

// NewRouter wires up all HTTP routes and returns the Gin engine.
func NewRouter(store TargetStore, cache StatusCache) *gin.Engine {
	r := gin.Default()

	h := newHandler(store, cache)

	// ── Frontend SPA ────────────────────────────────────────────────────────────
	// Served before API routes so the two explicit paths are registered as exact
	// matches and do not interfere with /api/v1/… or /swagger/… wildcards.
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/app.js", "./web/app.js")

	// ── System ──────────────────────────────────────────────────────────────────
	r.GET("/health", h.Health)

	// Swagger UI — available at /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── REST API v1 ─────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")
	{
		v1.GET("/targets", h.ListTargets)
		v1.POST("/targets", h.CreateTarget)
		v1.DELETE("/targets/:id", h.DeleteTarget)

		v1.GET("/targets/:id/status", h.GetStatus)
		v1.GET("/targets/:id/history", h.GetHistory)
		v1.GET("/targets/:id/stats", h.GetStats)
	}

	return r
}
