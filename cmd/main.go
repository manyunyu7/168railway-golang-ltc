package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/config"
	"github.com/modernland/golang-live-tracking/handlers"
	"github.com/modernland/golang-live-tracking/middleware"
	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/utils"
)

// compareVersions compares two semantic version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")
	
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	
	// Pad shorter version with zeros
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	
	for len(parts1) < maxLen {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < maxLen {
		parts2 = append(parts2, "0")
	}
	
	// Compare each part
	for i := 0; i < maxLen; i++ {
		n1, err1 := strconv.Atoi(parts1[i])
		n2, err2 := strconv.Atoi(parts2[i])
		
		// If conversion fails, treat as 0
		if err1 != nil {
			n1 = 0
		}
		if err2 != nil {
			n2 = 0
		}
		
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	
	return 0 // Equal
}

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Set Gin mode
	gin.SetMode(cfg.GinMode)

	// Connect to MySQL database
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUsername, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate only our session tracking table (skip Laravel tables)
	db.AutoMigrate(&models.LiveTrackingSession{})

	// Skip Redis for simple implementation
	// redisClient := redis.NewClient(&redis.Options{
	//     Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
	//     Password: cfg.RedisPassword,
	//     DB:       0,
	// })
	
	// Initialize S3 client
	s3Client := utils.NewS3Client(
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Region,
		cfg.S3Bucket,
		cfg.S3Endpoint,
	)

	// Initialize handlers and middleware
	authMiddleware := middleware.NewAuthMiddleware(db)
	// Use S3-enabled but Redis-free simple handler
	liveTrackingHandler := handlers.NewSimpleLiveTrackingHandler(db, s3Client)
	// Initialize WebSocket handler for real-time updates
	wsHandler := handlers.NewWebSocketHandler(db, s3Client)
	// Initialize API endpoints handler
	apiEndpointsHandler := handlers.NewAPIEndpointsHandler(db)
	// Initialize tile proxy handler for CartoDB tiles
	tileProxyHandler := handlers.NewTileProxyHandler()

	// Setup routes
	r := gin.Default()

	// Add comprehensive CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, Cache-Control, X-File-Name, X-CSRF-Token")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		quotes := []string{
			"The journey of a thousand miles begins with one step. - Lao Tzu",
			"Life is like riding a bicycle. To keep your balance, you must keep moving. - Albert Einstein",
			"It is during our darkest moments that we must focus to see the light. - Aristotle",
			"Success is not final, failure is not fatal: it is the courage to continue that counts. - Winston Churchill",
			"The only way to do great work is to love what you do. - Steve Jobs",
			"Innovation distinguishes between a leader and a follower. - Steve Jobs",
			"The future belongs to those who believe in the beauty of their dreams. - Eleanor Roosevelt",
			"It always seems impossible until it's done. - Nelson Mandela",
			"Don't watch the clock; do what it does. Keep going. - Sam Levenson",
			"Believe you can and you're halfway there. - Theodore Roosevelt",
		}
		
		randomQuote := quotes[rand.Intn(len(quotes))]
		
		c.JSON(200, gin.H{
			"status": "ok",
			"service": "golang-live-tracking",
			"quote": randomQuote,
		})
	})

	// WebSocket endpoint for real-time train updates
	r.GET("/ws/trains", wsHandler.HandleWebSocket)

	// API routes
	api := r.Group("/api")
	{
		// Public endpoints for train data (replace direct S3 access)
		api.GET("/active-train-list", liveTrackingHandler.GetActiveTrainsList)
		api.GET("/train/:trainNumber", liveTrackingHandler.GetTrainData)
		
		// Tile proxy endpoints for CartoDB maps (bypass blocking)
		api.GET("/tiles/:style/:z/:x/:y", tileProxyHandler.ProxyCartoDB)
		api.GET("/tiles/stats", tileProxyHandler.GetCacheStats)
		api.GET("/tiles/health", tileProxyHandler.HealthCheck)
		
		// Public API endpoints matching Laravel railway API
		api.GET("/stations", apiEndpointsHandler.GetStations)
		api.GET("/stations/:id", apiEndpointsHandler.GetStationByID)
		api.GET("/stations/search", apiEndpointsHandler.SearchStations)
		api.GET("/schedules", apiEndpointsHandler.GetSchedules)
		api.GET("/trains/:id/schedule", apiEndpointsHandler.GetTrainSchedule)
		api.GET("/operational-routes-pathway", apiEndpointsHandler.GetOperationalRoutesPathway)
		api.GET("/operational-routes/:id", apiEndpointsHandler.GetOperationalRouteByID)
		
		// Version control endpoints - platform-specific
		api.GET("/app-version", func(c *gin.Context) {
			platform := c.Query("platform") // ios or android
			
			versionConfig := utils.GetVersionConfig()
			
			if platform != "" {
				// Return platform-specific version
				platformConfig, err := utils.GetPlatformConfig(platform)
				if err != nil {
					c.JSON(400, gin.H{
						"success": false,
						"error": "Invalid platform",
						"message": "Supported platforms: ios, android",
					})
					return
				}
				
				c.JSON(200, gin.H{
					"platform":        platform,
					"current_version": platformConfig.CurrentVersion,
					"minimum_version": platformConfig.MinimumVersion,
					"force_update":    platformConfig.ForceUpdate,
					"update_message":  platformConfig.UpdateMessage,
					"download_url":    platformConfig.DownloadURL,
					"last_updated":    versionConfig.LastUpdated,
				})
			} else {
				// Return both platforms
				c.JSON(200, gin.H{
					"ios":          versionConfig.IOS,
					"android":      versionConfig.Android,
					"last_updated": versionConfig.LastUpdated,
				})
			}
		})
		
		// Version validation endpoint
		api.POST("/check-version", func(c *gin.Context) {
			var req struct {
				Version  string `json:"version" binding:"required"`
				Platform string `json:"platform"` // ios, android (optional for backward compatibility)
			}
			
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{
					"success": false,
					"error": "Invalid request format",
					"message": "Version parameter is required",
				})
				return
			}
			
			// Default to android if no platform specified (backward compatibility)
			platform := req.Platform
			if platform == "" {
				platform = "android"
			}
			
			platformConfig, err := utils.GetPlatformConfig(platform)
			if err != nil {
				c.JSON(400, gin.H{
					"success": false,
					"error": "Invalid platform",
					"message": "Supported platforms: ios, android",
				})
				return
			}
			
			currentVersion := platformConfig.CurrentVersion
			minimumVersion := platformConfig.MinimumVersion
			
			// Simple version comparison (assumes semantic versioning)
			isSupported := compareVersions(req.Version, minimumVersion) >= 0
			isLatest := compareVersions(req.Version, currentVersion) >= 0
			
			response := gin.H{
				"success": true,
				"supported": isSupported,
				"is_latest": isLatest,
				"platform": platform,
				"client_version": req.Version,
				"current_version": currentVersion,
				"minimum_version": minimumVersion,
			}
			
			if !isSupported {
				response["force_update"] = true
				response["message"] = "Your app version is no longer supported. Please update to continue using the service."
				response["download_url"] = platformConfig.DownloadURL
				c.JSON(200, response)
				return
			}
			
			if !isLatest {
				response["update_available"] = true
				response["message"] = "A new version is available with bug fixes and improvements!"
				response["download_url"] = "https://github.com/manyunyu7/168railway-golang-ltc/releases"
			} else {
				response["message"] = "You are using the latest version!"
			}
			
			c.JSON(200, response)
		})
		
		// Reload version config endpoint (admin use)
		api.POST("/reload-version-config", func(c *gin.Context) {
			config, err := utils.ReloadVersionConfig()
			if err != nil {
				c.JSON(500, gin.H{
					"success": false,
					"message": "Failed to reload version config",
					"error":   err.Error(),
				})
				return
			}
			
			c.JSON(200, gin.H{
				"success":      true,
				"message":      "Version configuration reloaded successfully",
				"ios":          config.IOS,
				"android":      config.Android,
				"last_updated": config.LastUpdated,
			})
		})
		
		// WebSocket upgrade information endpoint
		api.GET("/websocket-info", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"websocket_available": true,
				"websocket_url": "wss://go-ltc.trainradar35.com/ws/trains",
				"upgrade_benefits": []string{
					"Real-time updates every 5 seconds",
					"Individual passenger positions", 
					"Lower bandwidth usage",
					"Instant train status changes",
					"No polling required",
				},
				"http_endpoints_still_available": true,
				"migration_guide": "Connect to WebSocket for real-time data, keep HTTP as fallback",
			})
		})
		
		mobile := api.Group("/mobile")
		{
			// Protected live tracking routes
			liveTracking := mobile.Group("/live-tracking")
			liveTracking.Use(authMiddleware.SanctumAuth())
			{
				liveTracking.GET("/active-session", liveTrackingHandler.GetActiveSession)
				liveTracking.POST("/start", liveTrackingHandler.StartMobileSession)
				liveTracking.POST("/update", liveTrackingHandler.UpdateMobileLocation)
				liveTracking.POST("/heartbeat", liveTrackingHandler.Heartbeat)
				liveTracking.POST("/recover", liveTrackingHandler.RecoverSession)
				liveTracking.POST("/stop", liveTrackingHandler.StopMobileSession)
			}
		}
	}

	log.Printf("🚀 Golang Live Tracking API server starting on port %s", cfg.Port)
	log.Printf("📊 Database: %s@%s:%s/%s", cfg.DBUsername, cfg.DBHost, cfg.DBPort, cfg.DBName)
	log.Printf("☁️  S3: %s/%s", cfg.S3Endpoint, cfg.S3Bucket)
	log.Printf("⚡ Mode: S3-enabled but Redis-free implementation")

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}