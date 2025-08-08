# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a high-performance Golang replacement for Laravel live tracking API endpoints, designed to handle real-time GPS location tracking for train passengers. The application integrates with an existing Laravel system's database and validates Laravel Sanctum tokens for authentication.

**Production URL**: https://go-ltc.trainradar35.com/

**Current Implementation Status**: Simplified version without Redis/S3 dependencies for easier deployment. The production version uses database-only implementation with debug logging enabled.

## Core Architecture

- **Framework**: Gin (HTTP router) with GORM (ORM)
- **Database**: MySQL (shared with Laravel app)
- **Cache**: ~~Redis for session management~~ (Currently disabled)
- **Storage**: ~~S3-compatible storage (IDCloudHost) for train data files~~ (Currently disabled)
- **Authentication**: Laravel Sanctum token validation (SHA256 hashing)

### Key Components

- `cmd/main.go` - Application entry point, simplified implementation without Redis/S3
- `config/config.go` - Environment configuration loading with godotenv
- `middleware/auth.go` - Laravel Sanctum token authentication with debug logging
- `handlers/live_tracking.go` - Simplified API endpoints (using `NewSimpleLiveTrackingHandler`)
- `models/models.go` - Database models matching Laravel schema
- `utils/s3.go` - S3 client (currently unused in production)

### Data Flow (Current Simplified Implementation)

1. Mobile app gets Bearer token from `https://168railway.com/api/mobile/login` (Laravel)
2. Mobile app calls Golang API with Bearer token
3. Golang API validates token against shared `personal_access_tokens` table with debug logging
4. ~~User creates tracking session stored in Redis cache~~ (Currently returns no active sessions)
5. ~~Location updates stored as JSON files in S3~~ (Currently disabled)
6. ~~Session termination saves trip data to `trips` table~~ (Simplified responses)

**Important**: The Golang API shares the same MySQL database with the Laravel application but doesn't make HTTP requests to 168railway.com.

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

Configure environment variables:
- Database credentials (must match Laravel app's MySQL)
- ~~Redis connection details~~ (Currently unused)
- ~~S3 storage configuration~~ (Currently unused)
- Server port (default 8080)

**Production Environment**: The live server at https://go-ltc.trainradar35.com/ is configured with the necessary database connections.

## API Integration Details

### Authentication Flow
- Mobile apps obtain Bearer tokens from `https://168railway.com/api/mobile/login`
- Golang API validates tokens against shared Laravel `personal_access_tokens` table
- SHA256 hashes only the token part after "|" (Laravel Sanctum format: "id|token_string")
- Matches both token ID and hashed token in database
- Updates `last_used_at` timestamp on each request
- Validates token expiration if `expires_at` is set
- **Debug logging enabled** in production for troubleshooting
- **No HTTP calls to Laravel**: Direct database validation only

### Session Management (Currently Simplified)
- ~~Redis keys: `live_session_{sessionID}` and `user_sessions_{userID}`~~ (Disabled)
- ~~Session data includes train info, timestamps, S3 file paths~~ (Simplified)
- API endpoints return basic responses without full session tracking

### S3 Train Data Structure (Currently Disabled)
- ~~Files stored as `trains/train-{number}.json`~~ (Not implemented)
- ~~Contains passenger array with GPS coordinates, timestamps~~ (Not implemented)
- ~~Automatic cleanup when no passengers remain~~ (Not implemented)

## Database Schema Compatibility

Models are designed to match Laravel's database schema:
- `User` -> `users` table
- `PersonalAccessToken` -> `personal_access_tokens` table  
- `Trip` -> `trips` table with JSON columns for tracking data

## Performance Considerations

- ~~Redis caching for session lookups~~ (Currently disabled)
- ~~Efficient S3 operations with proper error handling~~ (Currently disabled)
- GORM optimizations with proper indexing
- Gin middleware for CORS and authentication
- Database auto-migration disabled to avoid key length issues with existing Laravel tables

## Health Monitoring

**Production Health Check**: https://go-ltc.trainradar35.com/health
- Returns `{"status": "ok", "service": "golang-live-tracking"}`
- Confirms service is operational and database connectivity

## Production Testing

Test authentication with curl:
```bash
curl -H "Authorization: Bearer YOUR_LARAVEL_SANCTUM_TOKEN" \
     -H "Content-Type: application/json" \
     https://go-ltc.trainradar35.com/api/mobile/live-tracking/active-session
```

**Note**: Debug logging is enabled in production, so server logs will show token validation details for troubleshooting.

## Available Endpoints

### Public Endpoints (No Authentication Required)

#### HTTP Endpoints
- `GET /health` - Health check endpoint with inspirational quotes
- `GET /api/active-train-list` - **New**: Public endpoint serving active trains list
  - Replaces direct S3 access: `https://is3.cloudhost.id/168railwaylivetracking/trains/trains-list.json`
  - Frontend should use: `https://go-ltc.trainradar35.com/api/active-train-list`
  - Returns JSON with active trains, passenger counts, and last update times
  - Includes proper CORS headers and cache control
- `GET /api/train/{trainNumber}` - **New**: Public endpoint serving individual train data
  - Replaces direct S3 access: `https://is3.cloudhost.id/168railwaylivetracking/trains/train-{trainNumber}.json`
  - Frontend should use: `https://go-ltc.trainradar35.com/api/train/{trainNumber}`
  - Returns individual train data with passengers, positions, and tracking info
  - Returns 404 if train not found

#### Version Control Endpoints
- `GET /api/app-version` - **New**: Get current app version information
  - Returns current version, minimum supported version, and update details
  - Used by mobile apps to check for available updates
- `POST /api/check-version` - **New**: Validate client app version compatibility
  - Request body: `{"version": "1.1.5"}`
  - Returns detailed compatibility information and update requirements
  - See [VERSION_API_GUIDE.md](VERSION_API_GUIDE.md) for complete documentation

#### WebSocket Endpoint (Real-time Updates) ⚡
- `WSS /ws/trains` - **New**: Real-time train tracking WebSocket
  - URL: `wss://go-ltc.trainradar35.com/ws/trains`
  - **Recommended for frontend** instead of HTTP polling
  - Broadcasts comprehensive train updates every 5 seconds
  - Includes individual passenger positions, not just train averages
  - Lower bandwidth than HTTP polling
  - Instant updates when trains move or passengers join/leave
  
**WebSocket Message Types:**
- `initial_data` - Full trains list on connection
- `train_updates` - Real-time updates with complete train data including:
  - Individual passenger locations and timestamps  
  - Average train position
  - Passenger count and status
  - Route information and data source
- `ping/pong` - Connection health checking

### Protected Endpoints (Require Laravel Sanctum Token)
All protected endpoints require `Authorization: Bearer {token}` header:

- `GET /api/mobile/live-tracking/active-session` - Check if user has active tracking session
- `POST /api/mobile/live-tracking/start` - Start new tracking session
- `POST /api/mobile/live-tracking/update` - Update GPS location during tracking
- `POST /api/mobile/live-tracking/heartbeat` - Send heartbeat to maintain session
- `POST /api/mobile/live-tracking/recover` - Recover lost session
- `POST /api/mobile/live-tracking/stop` - Stop tracking session