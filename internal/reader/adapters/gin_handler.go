package adapters

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/ports"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

type GinHandler struct {
	svc ports.QueryHandler
	log logger.Logger
}

func NewGinHandler(svc ports.QueryHandler, log logger.Logger) *GinHandler {
	return &GinHandler{
		svc: svc,
		log: *log.With(zap.String("adapter", "gin_handler")),
	}
}

type errorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// GetEvents handles GET /events
// @Summary      Query security events
// @Description  Returns filtered security events from InfluxDB
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        criticality  query     int                    true  "Minimum criticality (1-10)"   minimum(1)   maximum(10)
// @Param        limit        query     int                    true  "Maximum number of results (1-1000)" minimum(1) maximum(1000)
// @Success      200          {object}  queryResponseDTO
// @Failure      400          {object}  errorResponse
// @Failure      500          {object}  errorResponse
// @Router       /events [get]
func (h *GinHandler) GetEvents(c *gin.Context) {
	critStr := c.Query("criticality")
	limitStr := c.Query("limit")

	if critStr == "" || limitStr == "" {
		c.JSON(http.StatusBadRequest, errorResponse{
			Error: "criticality and limit query parameters are required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	crit, err := strconv.Atoi(critStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{
			Error: "criticality must be an integer",
			Code:  http.StatusBadRequest,
		})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{
			Error: "limit must be an integer",
			Code:  http.StatusBadRequest,
		})
		return
	}

	result, err := h.svc.HandleQuery(c.Request.Context(), crit, limit)
	if err != nil {
		if errors.Is(err, ports.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, errorResponse{
				Error: "invalid query parameters",
				Code:  http.StatusBadRequest,
			})
			return
		}
		h.log.Error("query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{
			Error: "internal server error",
			Code:  http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, toQueryResponseDTO(result))
}

// HealthHandler handles GET /health — used by Docker and Kubernetes health checks.
// @Summary      Health check
// @Description  Returns service health status
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (h *GinHandler) HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "reader",
	})
}

// ReadinessHandler handles GET /ready — checks downstream dependencies.
// @Summary      Readiness check
// @Description  Returns whether the service is ready to accept traffic
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /ready [get]
func (h *GinHandler) ReadinessHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ready",
		"service": "reader",
	})
}
