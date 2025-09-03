# Spotter Location API Documentation

## Overview

The Spotter Location API enables real-time tracking of users who are actively viewing the map. This feature allows train enthusiasts to see where other spotters are located, creating a community aspect to the train tracking experience.

**Base URL**: `https://go-ltc.trainradar35.com/api/spotters/`

## Features

- ✅ **Real-time location sharing** for active map users
- ✅ **Redis-backed caching** with 30-second updates
- ✅ **Auto-cleanup** after 5 minutes of inactivity
- ✅ **Scalable architecture** supporting 30,000+ concurrent users
- ✅ **Sanctum authentication** for secure access
- ✅ **Public access** for retrieving spotter locations

## Endpoints

### 1. Send Heartbeat (POST /heartbeat)

Send your current location while viewing the map.

**Endpoint**: `POST /api/spotters/heartbeat`  
**Authentication**: Required (Bearer Token)  
**Rate Limit**: 30 requests per hour per user

#### Request Headers
```http
Authorization: Bearer {sanctum_token}
Content-Type: application/json
```

#### Request Body
```json
{
  "latitude": -6.200000,
  "longitude": 106.816666
}
```

#### Parameters
| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `latitude` | float64 | Yes | -90 to 90 | User's latitude coordinate |
| `longitude` | float64 | Yes | -180 to 180 | User's longitude coordinate |

#### Success Response (200)
```json
{
  "success": true,
  "message": "Spotter location updated"
}
```

#### Error Responses

**401 Unauthorized - Missing Token**
```json
{
  "success": false,
  "message": "Authentication required - please provide Sanctum token"
}
```

**401 Unauthorized - Invalid Token**
```json
{
  "success": false,
  "message": "Invalid or expired token"
}
```

**400 Bad Request - Invalid Coordinates**
```json
{
  "success": false,
  "error": "Key: 'latitude' Error:Field validation for 'latitude' failed on the 'min' tag"
}
```

#### cURL Example
```bash
curl -X POST \
  -H "Authorization: Bearer 424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd" \
  -H "Content-Type: application/json" \
  -d '{"latitude": -6.200000, "longitude": 106.816666}' \
  https://go-ltc.trainradar35.com/api/spotters/heartbeat
```

---

### 2. Get Active Spotters (GET /active)

Retrieve all currently active spotters for map display.

**Endpoint**: `GET /api/spotters/active`  
**Authentication**: None (Public endpoint)  
**Cache**: 30-second cache, refreshed automatically

#### Request Headers
```http
Accept: application/json
```

#### Success Response (200)
```json
{
  "spotters": [
    {
      "user_id": 12,
      "username": "168Railway",
      "name": "Henry Augusta",
      "latitude": -6.200000,
      "longitude": 106.816666,
      "last_update": 1756874139155,
      "is_active": true
    },
    {
      "user_id": 25,
      "username": "trainspotter",
      "name": "John Doe",
      "latitude": -6.175000,
      "longitude": 106.827000,
      "last_update": 1756874098432,
      "is_active": true
    }
  ],
  "total": 2,
  "last_updated": "2025-09-03T04:35:50Z"
}
```

#### Response Fields
| Field | Type | Description |
|-------|------|-------------|
| `spotters` | array | List of active spotters |
| `spotters[].user_id` | integer | User's unique identifier |
| `spotters[].username` | string | User's display username |
| `spotters[].name` | string | User's full name |
| `spotters[].latitude` | float64 | Current latitude |
| `spotters[].longitude` | float64 | Current longitude |
| `spotters[].last_update` | integer | Unix timestamp (milliseconds) of last heartbeat |
| `spotters[].is_active` | boolean | Always true for active spotters |
| `total` | integer | Total number of active spotters |
| `last_updated` | string | ISO timestamp when response was generated |

#### cURL Example
```bash
curl -H "Accept: application/json" \
  https://go-ltc.trainradar35.com/api/spotters/active
```

---

## Authentication

### Sanctum Token Requirements

The heartbeat endpoint requires a valid Laravel Sanctum token obtained from your authentication system.

**Token Format**: `{token_id}|{token_string}`  
**Example**: `424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd`

### How to Obtain Tokens

#### For Mobile Apps
1. Login via `/api/mobile/login` endpoint
2. Use the returned Sanctum token for spotter API calls

#### For Web Apps
1. Login via Laravel web interface
2. Generate API token through user settings
3. Use token for spotter API calls

### Token Validation
- Tokens are validated against the `personal_access_tokens` table
- SHA256 hash matching for security
- Automatic `last_used_at` timestamp updates
- Expiration checking if `expires_at` is set

---

## Data Flow & Caching

### System Architecture
```
Client App → POST /heartbeat → Redis Cache → Background Updater (30s)
Map Frontend ← GET /active ← Cached Data ← Redis
```

### Cache Behavior
- **Write**: Immediate Redis storage with 5-minute TTL
- **Read**: 30-second cached responses for optimal performance
- **Cleanup**: Automatic removal after 5 minutes of inactivity
- **Background Process**: Cache updater runs every 30 seconds

### Performance Characteristics
- **Concurrent Users**: Supports 30,000+ active spotters
- **Response Time**: <100ms for cached responses
- **Update Frequency**: 30-second cache refresh cycle
- **Memory Usage**: ~190 bytes per active spotter

---

## Error Handling

### Common Error Codes

| Status Code | Description | Common Causes |
|-------------|-------------|---------------|
| 200 | Success | Request processed successfully |
| 400 | Bad Request | Invalid coordinates, malformed JSON |
| 401 | Unauthorized | Missing, invalid, or expired token |
| 500 | Internal Server Error | Redis unavailable, database issues |

### Best Practices

1. **Handle 401 errors** by redirecting to login
2. **Retry on 500 errors** with exponential backoff
3. **Validate coordinates** client-side before sending
4. **Cache responses** to reduce API calls

---

## Usage Patterns

### Recommended Flow

1. **User opens map** → Send initial heartbeat
2. **Every 2 minutes** → Send updated heartbeat
3. **User closes map** → Stop sending heartbeats
4. **Map display** → Fetch active spotters every 2 minutes

### Client Implementation

```javascript
// Example implementation
class SpotterLocationService {
  constructor(token) {
    this.token = token;
    this.heartbeatInterval = null;
    this.baseURL = 'https://go-ltc.trainradar35.com/api/spotters';
  }
  
  async startTracking(latitude, longitude) {
    // Send initial heartbeat
    await this.sendHeartbeat(latitude, longitude);
    
    // Start periodic heartbeats every 2 minutes
    this.heartbeatInterval = setInterval(() => {
      this.sendHeartbeat(latitude, longitude);
    }, 120000);
  }
  
  stopTracking() {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
      this.heartbeatInterval = null;
    }
  }
  
  async sendHeartbeat(latitude, longitude) {
    try {
      const response = await fetch(`${this.baseURL}/heartbeat`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${this.token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ latitude, longitude })
      });
      
      return await response.json();
    } catch (error) {
      console.error('Heartbeat failed:', error);
    }
  }
  
  async getActiveSpotters() {
    try {
      const response = await fetch(`${this.baseURL}/active`);
      return await response.json();
    } catch (error) {
      console.error('Failed to fetch spotters:', error);
    }
  }
}
```

---

## Rate Limiting & Quotas

### Current Limits
- **Heartbeat endpoint**: 30 requests per hour per user
- **Active spotters endpoint**: No rate limit (cached responses)

### Fair Usage Policy
- Send heartbeats only when actively viewing the map
- Maximum frequency: Every 2 minutes
- Stop heartbeats when map is closed
- Use cached responses when possible

---

## Monitoring & Health

### Health Check
```bash
curl https://go-ltc.trainradar35.com/health
```

### Debug Information
- Server logs include debug information for token validation
- Redis connection status available in health endpoint
- Cache hit rates monitored internally

---

## Migration Guide

### From Direct S3 Access
If you were previously accessing train data directly from S3, this new spotter API provides:

- **Better performance** through Redis caching
- **Real-time updates** every 30 seconds
- **Proper authentication** and rate limiting
- **Consistent API format** with other endpoints

### Frontend Integration
Replace direct S3 calls with API calls:

**Before:**
```javascript
fetch('https://is3.cloudhost.id/168railwaylivetracking/spotters/active.json')
```

**After:**
```javascript
fetch('https://go-ltc.trainradar35.com/api/spotters/active')
```

---

## Support & Contact

For questions or issues with the Spotter Location API:

- **GitHub Issues**: [Report bugs](https://github.com/manyunyu7/168railway-golang-ltc/issues)
- **API Status**: Monitor at `/health` endpoint
- **Documentation**: Always available at this location

---

*Last Updated: September 3, 2025*  
*API Version: 1.0.0*