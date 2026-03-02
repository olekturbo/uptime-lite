package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"uptime-lite/internal/models"
)

// TargetStore is the persistence interface required by the HTTP handlers.
type TargetStore interface {
	ListTargets(ctx context.Context) ([]models.Target, error)
	CreateTarget(ctx context.Context, t models.Target) (models.Target, error)
	DeleteTarget(ctx context.Context, id int64) error
	GetHistory(ctx context.Context, targetID int64, limit int) ([]models.CheckResult, error)
	GetUptimeStats(ctx context.Context, targetID int64, since time.Time) (float64, error)
}

// StatusCache is the cache interface required by the HTTP handlers.
type StatusCache interface {
	GetStatus(ctx context.Context, targetID int64) (*models.TargetStatus, error)
}

type handler struct {
	repo  TargetStore
	cache StatusCache
}

func newHandler(store TargetStore, cache StatusCache) *handler {
	return &handler{repo: store, cache: cache}
}

// errorResponse is the standard error envelope returned by all handlers.
type errorResponse struct {
	Error string `json:"error" example:"something went wrong"`
}

// statsResponse holds uptime percentage for a given target.
type statsResponse struct {
	TargetID    int64   `json:"target_id"    example:"1"`
	UptimePct   float64 `json:"uptime_pct"   example:"99.5"`
	PeriodHours int     `json:"period_hours" example:"24"`
}

// createTargetRequest is the request body for POST /api/v1/targets.
type createTargetRequest struct {
	Name            string `json:"name"             binding:"required"     example:"My API"`
	URL             string `json:"url"              binding:"required,url" example:"https://example.com/health"`
	IntervalSeconds int    `json:"interval_seconds" example:"60"`
	TimeoutSeconds  int    `json:"timeout_seconds"  example:"10"`
}

// Health godoc
//
//	@Summary	Health check
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	map[string]string	"ok"
//	@Router		/health [get]
func (h *handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListTargets godoc
//
//	@Summary	List all monitored targets
//	@Tags		targets
//	@Produce	json
//	@Success	200	{array}		models.Target
//	@Failure	500	{object}	errorResponse
//	@Router		/api/v1/targets [get]
func (h *handler) ListTargets(c *gin.Context) {
	targets, err := h.repo.ListTargets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, targets)
}

// CreateTarget godoc
//
//	@Summary	Add a new monitored target
//	@Tags		targets
//	@Accept		json
//	@Produce	json
//	@Param		target	body		createTargetRequest	true	"Target definition"
//	@Success	201		{object}	models.Target
//	@Failure	400		{object}	errorResponse
//	@Failure	500		{object}	errorResponse
//	@Router		/api/v1/targets [post]
func (h *handler) CreateTarget(c *gin.Context) {
	var req createTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if req.IntervalSeconds <= 0 {
		req.IntervalSeconds = 60
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 10
	}

	t := models.Target{
		Name:            req.Name,
		URL:             req.URL,
		IntervalSeconds: req.IntervalSeconds,
		TimeoutSeconds:  req.TimeoutSeconds,
		Active:          true,
	}

	created, err := h.repo.CreateTarget(c.Request.Context(), t)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

// DeleteTarget godoc
//
//	@Summary	Delete a target and its history
//	@Tags		targets
//	@Param		id	path	int	true	"Target ID"
//	@Success	204
//	@Failure	400	{object}	errorResponse
//	@Failure	500	{object}	errorResponse
//	@Router		/api/v1/targets/{id} [delete]
func (h *handler) DeleteTarget(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid id"})
		return
	}
	if err := h.repo.DeleteTarget(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetStatus godoc
//
//	@Summary	Get the latest probe result (served from Redis cache)
//	@Tags		targets
//	@Produce	json
//	@Param		id	path		int	true	"Target ID"
//	@Success	200	{object}	models.TargetStatus
//	@Failure	400	{object}	errorResponse
//	@Failure	404	{object}	errorResponse
//	@Failure	500	{object}	errorResponse
//	@Router		/api/v1/targets/{id}/status [get]
func (h *handler) GetStatus(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid id"})
		return
	}

	status, err := h.cache.GetStatus(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if status == nil {
		c.JSON(http.StatusNotFound, errorResponse{Error: "no status available yet; check back after the first probe"})
		return
	}
	c.JSON(http.StatusOK, status)
}

// GetHistory godoc
//
//	@Summary	Get probe history for a target
//	@Tags		targets
//	@Produce	json
//	@Param		id		path		int	true	"Target ID"
//	@Param		limit	query		int	false	"Max number of results (1–1000, default 100)"
//	@Success	200		{array}		models.CheckResult
//	@Failure	400		{object}	errorResponse
//	@Failure	500		{object}	errorResponse
//	@Router		/api/v1/targets/{id}/history [get]
func (h *handler) GetHistory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid id"})
		return
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	results, err := h.repo.GetHistory(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

// GetStats godoc
//
//	@Summary	Get uptime percentage for the last 24 hours
//	@Tags		targets
//	@Produce	json
//	@Param		id	path		int	true	"Target ID"
//	@Success	200	{object}	statsResponse
//	@Failure	400	{object}	errorResponse
//	@Failure	500	{object}	errorResponse
//	@Router		/api/v1/targets/{id}/stats [get]
func (h *handler) GetStats(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid id"})
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	uptime, err := h.repo.GetUptimeStats(c.Request.Context(), id, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, statsResponse{
		TargetID:    id,
		UptimePct:   uptime,
		PeriodHours: 24,
	})
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}
