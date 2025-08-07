# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a high-performance Golang replacement for Laravel live tracking API endpoints, designed to handle real-time GPS location tracking for train passengers. The application integrates with an existing Laravel system's database and validates Laravel Sanctum tokens for authentication.

## Core Architecture

- **Framework**: Gin (HTTP router) with GORM (ORM)
- **Database**: MySQL (shared with Laravel app)
- **Cache**: Redis for session management
- **Storage**: S3-compatible storage (IDCloudHost) for train data files
- **Authentication**: Laravel Sanctum token validation (SHA256 hashing)

### Key Components

- `cmd/main.go` - Application entry point, initializes all services
- `config/config.go` - Environment configuration loading with godotenv
- `middleware/auth.go` - Laravel Sanctum token authentication
- `handlers/live_tracking.go` - Main API endpoints for location tracking
- `models/models.go` - Database models matching Laravel schema
- `utils/s3.go` - S3 client for train data file operations

### Data Flow

1. Mobile app sends Bearer token (Laravel Sanctum)
2. Middleware validates token against `personal_access_tokens` table  
3. User creates tracking session stored in Redis cache
4. Location updates stored as JSON files in S3 (`trains/train-{number}.json`)
5. Session termination saves trip data to `trips` table

## Common Development Commands

### Build and Run
```bash
# Install dependencies
go mod tidy

# Run in development mode
go run cmd/main.go

# Build binary
go build -o bin/golang-live-tracking cmd/main.go

# Run built binary
./bin/golang-live-tracking
```

### Docker
```bash
# Build image
docker build -t golang-live-tracking .

# Run container
docker run -p 8080:8080 --env-file .env golang-live-tracking
```

### Testing
```bash
# Run tests (if any exist)
go test ./...

# Test specific package
go test ./handlers

# Run with coverage
go test -cover ./...
```

## Environment Configuration

Copy `.env.example` to `.env` and configure:
- Database credentials (must match Laravel app's MySQL)
- Redis connection details
- S3 storage configuration
- Server port (default 8080)

## API Integration Details

### Authentication Flow
- Uses Laravel's `personal_access_tokens` table
- SHA256 hashes plain-text tokens from Authorization header
- Updates `last_used_at` timestamp on each request
- Validates token expiration if `expires_at` is set

### Session Management
- Redis keys: `live_session_{sessionID}` and `user_sessions_{userID}`
- Session data includes train info, timestamps, S3 file paths
- 24-hour expiration with heartbeat updates

### S3 Train Data Structure
- Files stored as `trains/train-{number}.json`
- Contains passenger array with GPS coordinates, timestamps
- Automatic cleanup when no passengers remain
- Average position calculated from active passengers

## Database Schema Compatibility

Models are designed to match Laravel's database schema:
- `User` -> `users` table
- `PersonalAccessToken` -> `personal_access_tokens` table  
- `Trip` -> `trips` table with JSON columns for tracking data

## Performance Considerations

- Redis caching for session lookups
- Efficient S3 operations with proper error handling
- GORM optimizations with proper indexing
- Gin middleware for CORS and authentication
- Graceful passenger filtering based on client type timeouts

## Health Monitoring

Health check endpoint available at `/health` returns service status and configuration summary.