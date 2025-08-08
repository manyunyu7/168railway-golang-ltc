package config

import (
	"os"
	"github.com/joho/godotenv"
)

type Config struct {
	// Database
	DBHost     string
	DBPort     string
	DBUsername string
	DBPassword string
	DBName     string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string
	
	// App Version
	CurrentVersion string
	MinimumVersion string

	// Server
	Port    string
	GinMode string

	// Laravel Integration
	LaravelAppKey     string
	SanctumTokenPrefix string

	// S3
	S3AccessKey string
	S3SecretKey string
	S3Region    string
	S3Bucket    string
	S3Endpoint  string
}

func LoadConfig() *Config {
	godotenv.Load()

	return &Config{
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "3306"),
		DBUsername:        getEnv("DB_USERNAME", "root"),
		DBPassword:        getEnv("DB_PASSWORD", ""),
		DBName:            getEnv("DB_NAME", "database"),
		RedisHost:         getEnv("REDIS_HOST", "localhost"),
		RedisPort:         getEnv("REDIS_PORT", "6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		Port:              getEnv("PORT", "8080"),
		GinMode:           getEnv("GIN_MODE", "debug"),
		CurrentVersion:    getEnv("APP_CURRENT_VERSION", "1.2.0"),
		MinimumVersion:    getEnv("APP_MINIMUM_VERSION", "1.1.0"),
		LaravelAppKey:     getEnv("LARAVEL_APP_KEY", ""),
		SanctumTokenPrefix: getEnv("SANCTUM_TOKEN_PREFIX", ""),
		S3AccessKey:       getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:       getEnv("S3_SECRET_KEY", ""),
		S3Region:          getEnv("S3_REGION", ""),
		S3Bucket:          getEnv("S3_BUCKET", ""),
		S3Endpoint:        getEnv("S3_ENDPOINT", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}