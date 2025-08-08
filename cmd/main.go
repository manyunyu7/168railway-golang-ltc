package main

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/config"
	"github.com/modernland/golang-live-tracking/handlers"
	"github.com/modernland/golang-live-tracking/middleware"
	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/utils"
)

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

	// Setup routes
	r := gin.Default()

	// Add CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
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