package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/config"
	"github.com/modernland/golang-live-tracking/handlers"
	"github.com/modernland/golang-live-tracking/middleware"
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

	// Skip auto migration to avoid key length issues - use existing Laravel tables
	// db.AutoMigrate(&models.User{}, &models.PersonalAccessToken{}, &models.Trip{})

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
		c.JSON(200, gin.H{
			"status": "ok",
			"service": "golang-live-tracking",
		})
	})

	// API routes
	api := r.Group("/api")
	{
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