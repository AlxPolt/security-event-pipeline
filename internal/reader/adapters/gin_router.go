package adapters

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	_ "github.com/AlxPolt/sw-engineer-challenge/docs" // Swagger generated docs
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

// NewRouter constructs the Gin router with all middleware and routes registered.
// The router is returned so cmd/reader/main.go can configure TLS and the
// http.Server — separation of routing from transport concerns.
//
// allowedOrigins is the CORS allowlist loaded from CORS_ALLOWED_ORIGINS env.
// In development mode (env="development") all origins are permitted regardless
// of the allowlist. In production, only listed origins receive CORS headers.
func NewRouter(handler *GinHandler, log logger.Logger, env string, allowedOrigins []string) *gin.Engine {
	if env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New() // gin.New() instead of Default() — we add our own middleware

	// Middleware

	// 1. Recovery — converts panics to 500s. We still don't panic intentionally,
	//    but third-party libraries may, and we must not crash the entire service.
	router.Use(gin.Recovery())

	// 2. Structured request logging.
	router.Use(structuredLogger(log))

	// 3. Security headers (OWASP ASVS V14.4).
	router.Use(securityHeaders())

	// 4. CORS policy — allowlist driven by config, not hardcoded.
	router.Use(corsPolicy(env, allowedOrigins))

	// Routes

	// Secure group — strict CSP applied to all API and health routes.
	secure := router.Group("")
	secure.Use(securityHeaders())

	// Health endpoints — no auth required, used by load balancers.
	secure.GET("/health", handler.HealthHandler)
	secure.GET("/ready", handler.ReadinessHandler)

	// API v1 group.
	v1 := secure.Group("/api/v1")
	{
		v1.GET("/events", handler.GetEvents)
	}

	if env != "production" {
		swaggerGroup := router.Group("")
		swaggerGroup.Use(swaggerSecurityHeaders())
		swaggerGroup.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	return router
}

func structuredLogger(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		log.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

// securityHeaders sets HTTP security headers on every response.
// Aligned with OWASP Secure Headers Project and ASVS V14.4.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Remove server fingerprinting headers.
		c.Header("Server", "")
		c.Next()
	}
}

// swaggerSecurityHeaders provides a relaxed CSP for the Swagger UI.
// Swagger requires inline scripts and styles to function.
func swaggerSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")

		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Server", "")
		c.Next()
	}
}

func corsPolicy(env string, allowedOrigins []string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		set[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := isAllowedOrigin(origin, env, set)

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Max-Age", "3600")
		}

		if c.Request.Method == http.MethodOptions {
			if allowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}

		c.Next()
	}
}

func isAllowedOrigin(origin, env string, allowlist map[string]struct{}) bool {
	if env == "development" {
		return true // Allow all origins in development
	}
	if origin == "" {
		return false
	}
	_, ok := allowlist[origin]
	return ok
}
