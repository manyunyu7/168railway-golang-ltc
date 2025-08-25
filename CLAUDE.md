# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a high-performance Golang replacement for Laravel live tracking API endpoints, designed to handle real-time GPS location tracking for train passengers. The application integrates with an existing Laravel system's database and validates Laravel Sanctum tokens for authentication.

**Production URL**: https://go-ltc.trainradar35.com/

**Current Implementation Status**: Database + S3 implementation with Redis disabled for easier deployment. The production version uses S3 for train data storage with database session tracking and debug logging enabled.

## Core Architecture

- **Framework**: Gin (HTTP router) with GORM (ORM)
- **Database**: MySQL (shared with Laravel app)
- **Cache**: ~~Redis for session management~~ (Currently disabled)
- **Storage**: S3-compatible storage (IDCloudHost) for train data files
- **Authentication**: Laravel Sanctum token validation (SHA256 hashing)

### Key Components

- `cmd/main.go` - Application entry point, S3-enabled implementation without Redis
- `config/config.go` - Environment configuration loading with godotenv
- `middleware/auth.go` - Laravel Sanctum token authentication with debug logging
- `handlers/simple_live_tracking.go` - Live tracking API endpoints (using `NewSimpleLiveTrackingHandler`)
- `models/models.go` - Database models matching Laravel schema
- `utils/s3.go` - S3 client for train data storage

### Data Flow (Current S3-Enabled Implementation)

1. Mobile app gets Bearer token from `https://168railway.com/api/mobile/login` (Laravel)
2. Mobile app calls Golang API with Bearer token
3. Golang API validates token against shared `personal_access_tokens` table with debug logging
4. User creates tracking session stored in database (replaces Redis cache)
5. Location updates stored as JSON files in S3 with real-time updates
6. Session termination saves trip data to `trips` table with complete GPS tracking history

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

### Production Deployment
```bash
# 1. Stash any local changes (server is only for deployment, not development)
git stash --all

# 2. Pull latest code from repository
git pull

# 3. Rebuild the binary (Go is compiled, not interpreted)
go build -o go-ltc cmd/main.go

# 4. Restart the systemd service (NOT manual process)
sudo systemctl restart go-ltc

# 5. Check if service is running properly
sudo systemctl status go-ltc

# 6. Test the health endpoint
curl https://go-ltc.trainradar35.com/health
```

**Important Deployment Notes**:
- **Environment Configuration**: The `.env` file must exist at `/var/www/go-ltc/.env` with correct database credentials (DB_USERNAME, DB_PASSWORD, DB_NAME, etc.) and S3 configuration (S3_ACCESS_KEY, S3_SECRET_KEY, S3_REGION, S3_BUCKET, S3_ENDPOINT)
- **Service Management**: Always use systemd to manage the service (`sudo systemctl restart go-ltc`), NOT manual nohup commands
- **Build Required**: Go is a compiled language - changes won't take effect until you rebuild with `go build` AND restart the service
- **Check Logs**: If issues occur, check systemd logs with `sudo journalctl -u go-ltc -n 50`
- **Silent Success**: When `go build` runs without output, it means the build succeeded (Go only shows errors, not success messages)

**Common Issues**:
- If service fails to start: Check `.env` file exists and has correct DB_USERNAME (not DB_USER) and S3 configuration
- If endpoints return 404: Ensure you've rebuilt the binary and restarted the service
- Database connection errors: Verify `.env` has correct credentials matching the Laravel application's database
- S3 "MissingRegion" errors: Ensure S3_REGION is set in `.env` file (e.g., S3_REGION=us-east-1)

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
- S3 storage configuration (S3_ACCESS_KEY, S3_SECRET_KEY, S3_REGION, S3_BUCKET, S3_ENDPOINT)
- Server port (default 8080)

**Production Environment**: The live server at https://go-ltc.trainradar35.com/ is configured with the necessary database connections and S3 storage.

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

### Session Management (Database-backed)
- ~~Redis keys: `live_session_{sessionID}` and `user_sessions_{userID}`~~ (Replaced with database)
- Session data stored in `live_tracking_sessions` table with train info, timestamps, S3 file paths
- API endpoints provide full session tracking with S3 integration

### S3 Train Data Structure
- Files stored as `trains/train-{number}.json` with real-time passenger data
- Contains passenger array with GPS coordinates, timestamps, and tracking details
- Automatic cleanup when no passengers remain on the train
- Active trains list maintained in `trains/trains-list.json`

## Database Schema Compatibility

Models are designed to match Laravel's database schema:
- `User` -> `users` table
- `PersonalAccessToken` -> `personal_access_tokens` table  
- `Trip` -> `trips` table with JSON columns for tracking data

## Performance Considerations

- ~~Redis caching for session lookups~~ (Replaced with database sessions)
- Efficient S3 operations with proper error handling for train data storage
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
  - Individual passenger locations, timestamps, and speed data
  - Average train position and speed
  - Passenger count and status
  - Route information and data source
- `ping/pong` - Connection health checking

### Protected Endpoints (Require Laravel Sanctum Token)
All protected endpoints require `Authorization: Bearer {token}` header:

- `GET /api/mobile/live-tracking/active-session` - Check if user has active tracking session
  - Returns `session_status` field: `"active"`, `"terminated"`, `"inactive"`, or `"none"`
- `POST /api/mobile/live-tracking/start` - Start new tracking session
- `POST /api/mobile/live-tracking/update` - Update GPS location during tracking
  - **New**: Returns `session_status` field to inform mobile apps of session state
  - If session is terminated: `success: false`, `session_status: "terminated"`
  - If session is active: `success: true`, `session_status: "active"`
- `POST /api/mobile/live-tracking/heartbeat` - Send heartbeat to maintain session  
  - **New**: Returns `session_status` field and updates heartbeat only for active sessions
  - Returns `success: false` if session is terminated/inactive
- `POST /api/mobile/live-tracking/recover` - Recover lost session
- `POST /api/mobile/live-tracking/stop` - Stop tracking session
  - **New**: Users can save their journey even after admin termination
  - Returns `was_terminated_by_admin: true` if session was terminated by admin
  - Session status becomes `"terminated_with_trip_saved"` when trip is saved after termination

#### Session Status Values
Mobile apps can check the `session_status` field in API responses:
- `"active"` - Session is running normally, GPS updates accepted
- `"terminated"` - Session terminated by admin, GPS updates ignored
- `"terminated_with_trip_saved"` - Session terminated by admin, but user saved their trip ✅
- `"completed"` - Session stopped normally by user
- `"inactive"` - Session expired or stopped by user
- `"none"` - No active session found for user
- `"not_found"` - Session ID not found in database

### Admin Interface Endpoints (Session-Based Authentication)
**Web Interface:**
- `GET /admin/login` - Admin login page (mobile-responsive, 168railway.com inspired design)
- `POST /admin/login` - Handle admin login form submission  
- `GET /admin/dashboard` - Main admin dashboard (real-time session monitoring)
- `GET /admin/logout` - Admin logout and session cleanup

**Admin API (requires admin session cookie):**
- `GET /admin/api/sessions` - List live tracking sessions with filtering
  - Parameters: `status=active|all|inactive|terminated`, `limit=25|50|100`
- `POST /admin/api/sessions/terminate/{session_id}` - Terminate a user's active session
  - Returns success/failure with detailed session information

## Admin Interface 🔐

The application includes a comprehensive web-based admin dashboard for monitoring and managing live tracking sessions.

**Access URL**: https://go-ltc.trainradar35.com/admin/dashboard

### Admin Authentication
- **Login Page**: Simple, gradient-free design optimized for mobile devices
- **Authentication**: Database-based admin session management (in-memory)
- **Session Duration**: 24 hours with automatic expiration
- **Access Control**: Only users with `role = 'admin'` in the database can log in

### Admin Dashboard Features
- **Mobile-Responsive Design**: Optimized for tablets and smartphones
- **Real-time Statistics**: Active sessions, trains, users, and system status
- **Live Session Monitoring**: View all active tracking sessions with user details
- **Session Management**: Ability to terminate active sessions
- **Auto-refresh**: Optional 5-second auto-refresh for real-time monitoring
- **Modern UI**: Clean, professional interface with animations and status indicators

### Admin Endpoints
All admin endpoints require authentication via admin session cookie:

#### Web Interface Routes
- `GET /admin/login` - Admin login page (mobile-responsive, no gradients)
- `POST /admin/login` - Handle admin login form submission
- `GET /admin/dashboard` - Main admin dashboard (mobile-friendly)
- `GET /admin/logout` - Admin logout and session cleanup

#### Admin API Routes (Session-Based Authentication)
- `GET /admin/api/sessions` - List tracking sessions with filtering
  - Query parameters: `status` (active/all/inactive/terminated), `limit` (25/50/100)
  - Returns formatted session data with user information
- `POST /admin/api/sessions/terminate/{session_id}` - Terminate a specific session
  - Requires confirmation before termination
  - Updates session status to "terminated" in database

### Admin Dashboard Interface
The dashboard provides:
- **Statistics Cards**: Real-time metrics with animated counters
- **Session Table**: Comprehensive view of all tracking sessions
- **Advanced Filters**: Status-based filtering and pagination
- **User Information**: Displays usernames, station names, and avatars
- **Train Details**: Shows train numbers and route information
- **Action Controls**: Terminate sessions with confirmation dialogs
- **System Health**: Live system status monitoring

### Mobile Optimization
The admin interface is fully responsive with:
- **Tablet Layout**: Optimized for tablets (768px and below)
- **Mobile Layout**: Mobile-friendly design (576px and below)
- **Compact Tables**: Hidden columns on small screens for better usability  
- **Touch-Friendly**: Large buttons and touch targets
- **Adaptive Text**: Scalable fonts and spacing
- **Notification System**: Mobile-optimized alert positioning

### Admin Development Notes
- **Session Storage**: Currently uses in-memory sessions (consider Redis for production scaling)
- **Password Authentication**: Supports both Laravel bcrypt hashes and fallback passwords for development
- **Debug Logging**: All admin actions are logged with timestamps and user info
- **Security**: Secure session cookies with httpOnly flag
- **Error Handling**: Comprehensive error messages and user feedback

### Admin Testing
Test admin login with curl:
```bash
# Test admin session API (requires valid admin session cookie)
curl -b "admin_session=VALID_SESSION_ID" \
     https://go-ltc.trainradar35.com/admin/api/sessions?status=active&limit=25
```

**Admin Credentials**: Use any user in the database with `role = 'admin'` to access the admin interface.

## Recent Updates (2025-08-25) 🆕

### Admin Interface Implementation
- **New Web Admin Dashboard**: Complete administrative interface for managing live tracking sessions
  - **URL**: `https://go-ltc.trainradar35.com/admin/dashboard`  
  - **Mobile-Responsive Design**: Fully optimized for tablets and smartphones with Bootstrap 5.3.2
  - **Modern Login Page**: Clean, professional design inspired by 168railway.com using Inter font
  - **Real-time Monitoring**: Live session tracking with auto-refresh capability
  - **Session Management**: Ability to terminate active user sessions with confirmation

### Session Status API Enhancement
- **New `session_status` Field**: All mobile API endpoints now return session status information
  - **GPS Update Endpoint**: Returns `"terminated"` when admin terminates session
  - **Heartbeat Endpoint**: Updates heartbeat only for active sessions, returns status
  - **Active Session Check**: Includes current session status in response
  
### API Response Changes
**Before (Silent Failure):**
```json
{
  "success": true,
  "message": "Mobile location updated successfully",
  "updated_file": ""
}
```

**After (Clear Status):**
```json
{
  "success": false,
  "message": "Session is no longer active", 
  "updated_file": "",
  "session_status": "terminated"
}
```

### Admin Features
- **Session Termination**: Admins can terminate user sessions with immediate effect
- **Real-time Dashboard**: Live statistics and session monitoring
- **Mobile-Friendly Interface**: Responsive design works on all devices
- **User Management**: View user details, station names, and session history
- **System Health Monitoring**: Live system status and health checks

### Technical Improvements
- **Enhanced Error Handling**: Better session state management and validation
- **Improved Mobile UX**: Apps can now detect and handle terminated sessions gracefully
- **Debug Logging**: Comprehensive logging for session termination and status changes
- **Database-Driven**: All session management now uses database as single source of truth
- **WebSocket Speed Data**: Added average speed calculation and individual passenger speed preservation

### Files Added/Modified
- `handlers/web_admin.go` - Complete admin interface implementation
- `handlers/admin.go` - Admin API endpoints
- `middleware/admin.go` - Admin authentication middleware
- `handlers/simple_live_tracking.go` - Enhanced with session status responses
- `CLAUDE.md` - Updated documentation

### Trip Saving After Admin Termination ✅
- **Important**: Users can still save their journey even if admin terminates their session
- **Stop Session Endpoint**: Now accepts both `"active"` and `"terminated"` sessions for trip saving
- **Special Status**: Sessions become `"terminated_with_trip_saved"` when user saves after termination
- **Response Fields**: `was_terminated_by_admin: true` indicates admin termination

### Breaking Changes for Mobile Apps
- **GPS Update Response**: Now returns `success: false` when session is terminated (previously `true`)
- **New Required Field**: Mobile apps should check `session_status` field in all responses
- **Heartbeat Changes**: Now validates session status and updates heartbeat conditionally
- **Trip Saving**: Users can now save trips after admin termination (NEW feature)

### Deployment Notes
- **Service Restart Required**: Changes require `go build` and `sudo systemctl restart go-ltc`
- **Database Compatible**: No schema changes, uses existing `live_tracking_sessions` table
- **Backward Compatible**: Existing mobile apps will continue to work but won't utilize new status features