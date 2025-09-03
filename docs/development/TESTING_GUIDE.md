# 🧪 Testing Guide for Golang Live Tracking API

## 📋 Prerequisites

1. **Start the Golang API server:**
   ```bash
   cd /path/to/golang-live-tracking
   go run cmd/main.go
   ```
   Server will start on `http://localhost:8080`

2. **Import Postman Collection:**
   - Open Postman
   - Import `postman-collection.json`
   - Collection includes all endpoints and test cases

## 🔐 Authentication Setup

### Method 1: Generate Token via Laravel Artisan (Recommended)
```bash
cd /path/to/dashboard_gapeka
php artisan api:generate-token henryaugusta8@gmail.com --name="Golang_Testing"
```

Copy the generated Bearer token (format: `1|abc123...`)

### Method 2: Use Laravel Login API
1. Use the "Generate Bearer Token (Laravel)" request in Postman
2. Login with:
   ```json
   {
     "email": "henryaugusta8@gmail.com", 
     "password": "password"
   }
   ```
3. Extract `access_token` from response

### Method 3: Use Existing Token
If you have an existing mobile app token, you can use it directly.

## 🚀 Testing Flow

### 1. **Health Check**
```bash
GET http://localhost:8080/health
```
Should return: `{"status": "ok", "service": "golang-live-tracking"}`

### 2. **Update Bearer Token**
- Set the `bearer_token` collection variable in Postman
- Replace `PASTE_TOKEN_HERE` with your actual token

### 3. **Test Complete Flow**
Run requests in this order:

1. **Get Active Session** - Should return `has_active_session: false`
2. **Start Mobile Session** - Creates new session (auto-saves session_id)
3. **Update Mobile Location** - Updates GPS coordinates  
4. **Send Heartbeat** - Keeps session alive
5. **Recover Session** - Test reconnection
6. **Stop Session** - Choose with or without trip save

## 📊 Expected Responses

### ✅ Success Cases

**Start Session:**
```json
{
  "success": true,
  "session_id": "uuid-here",
  "message": "Mobile tracking session started successfully"
}
```

**Update Location:**
```json
{
  "success": true,
  "message": "Mobile location updated successfully"
}
```

**Stop with Trip Save:**
```json
{
  "success": true,
  "message": "Mobile tracking session stopped successfully", 
  "trip_saved": true,
  "trip_id": 123
}
```

### ❌ Error Cases

**Invalid Token (401):**
```json
{
  "success": false,
  "message": "Invalid or expired token"
}
```

**Invalid Session (403):**
```json
{
  "success": false,
  "message": "Invalid session"
}
```

## 🔍 Testing Scenarios

### Authentication Tests
- ✅ Valid Bearer token
- ❌ Invalid token format
- ❌ Expired token  
- ❌ Missing Authorization header

### Session Management Tests
- ✅ Start new session
- ✅ Update existing session
- ❌ Update non-existent session
- ❌ Use another user's session
- ✅ Multiple updates in sequence
- ✅ Session recovery after disconnect

### Data Validation Tests  
- ❌ Invalid latitude/longitude ranges
- ❌ Malformed UUID session_id
- ❌ Missing required fields
- ✅ Optional fields (accuracy, speed, etc.)

### Performance Tests
- 🚀 Response time < 100ms for most endpoints
- 🚀 Concurrent requests handling
- 🚀 Memory usage monitoring

## 📈 Monitoring

### Check Server Logs
```bash
# If running with go run
# Logs appear in terminal

# Check database connections
# Redis connections
# S3 upload status
```

### Database Verification
```sql
-- Check if trip was saved
SELECT * FROM trips ORDER BY id DESC LIMIT 1;

-- Check user sessions
SELECT * FROM personal_access_tokens WHERE last_used_at IS NOT NULL ORDER BY last_used_at DESC;

-- Check users
SELECT id, name, email FROM users WHERE email = 'henryaugusta8@gmail.com';
```

### S3 Storage Check
- Verify train files are created in S3: `trains/train-{number}.json`
- Check trains-list.json is updated
- Confirm file cleanup on session end

## 🐛 Troubleshooting

### Common Issues

**"Failed to connect to database"**
- Check MySQL credentials in .env
- Ensure database server is accessible
- Verify database exists

**"Invalid or expired token"**  
- Generate new token via artisan command
- Check token format (should include `|`)
- Verify user exists in database

**"Failed to start tracking session"**
- Check S3 credentials and permissions
- Verify bucket exists and is accessible
- Check network connectivity to S3 endpoint

**Redis Connection Issues**
- Ensure Redis server is running
- Check Redis host/port configuration
- Test Redis connectivity: `redis-cli ping`

### Debug Mode
Set `GIN_MODE=debug` in .env for detailed logging.

## 🔄 Comparison with Laravel

Test same endpoints on Laravel for comparison:
- Laravel: `https://168railway.com/api/mobile/live-tracking/*`
- Golang: `http://localhost:8080/api/mobile/live-tracking/*`

Expected improvements:
- ⚡ ~10x faster response times
- 📉 Lower memory usage
- 🔄 Better concurrent request handling

## 📝 Notes

- Session IDs are auto-extracted and stored in Postman variables
- All timestamps are in UTC
- GPS coordinates use Jakarta area examples
- Trip statistics include realistic train journey data