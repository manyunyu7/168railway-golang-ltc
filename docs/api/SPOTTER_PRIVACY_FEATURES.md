# Spotter Location Privacy Features

## Overview

The spotter location system now includes comprehensive privacy controls that allow users to control their visibility on the map. These features provide two levels of privacy protection while maintaining admin oversight capabilities.

## Privacy Options

### 1. Hide Location Completely (`hide_location: true`)
- **Effect**: User disappears completely from public map
- **Visibility**: Only visible to admins via admin endpoint
- **Use Case**: Users who want complete privacy while still receiving map features

### 2. Hide Identity (`hide_identity: true`)  
- **Effect**: User location is visible but shows as "Anonymous User"
- **Visibility**: Location visible to all, identity hidden from public
- **Admin View**: Admins can see real identity and user details
- **Use Case**: Users who want to contribute to map activity but remain anonymous

### 3. No Privacy Settings (default)
- **Effect**: Full visibility with username displayed
- **Visibility**: Username and location visible to all users
- **Use Case**: Users comfortable with public visibility

## API Endpoints

### Unified Endpoint (Role-Based Response)
**URL**: `GET /api/spotters/active`

**Authentication**: **No token required** - Public endpoint with optional admin enhancement

**Behavior**: Automatically detects user role from optional Sanctum token
- **No Token (Anonymous)**: ✅ Returns public filtered results
- **Regular User Token**: Returns same public filtered results  
- **Admin User Token**: Returns full unfiltered admin data

**Access Level**:
- **Public Access**: Always available without authentication
- **Admin Enhancement**: Provide `Authorization: Bearer {admin_token}` header to get full data

**Public User Response**: Respects all privacy settings
```json
{
  "spotters": [
    {
      "user_id": 123,
      "username": "TrainSpotter01",
      "latitude": -6.2,
      "longitude": 106.8,
      "last_update": 1756878189355,
      "is_active": true
    },
    {
      "username": "Anonymous User",
      "latitude": -6.17,
      "longitude": 106.82,
      "last_update": 1756878146383,
      "is_active": true
    }
  ],
  "total": 2,
  "last_updated": "2025-09-03T05:44:58Z"
}
```

**Admin User Response**: Shows all users including hidden ones  
**Auth**: Optional Sanctum token with `role = 'admin'` (if provided)
```json
{
  "spotters": [
    {
      "user_id": 123,
      "username": "TrainSpotter01", 
      "name": "John Doe",
      "latitude": -6.2,
      "longitude": 106.8,
      "last_update": 1756878189355,
      "is_active": true,
      "hide_location": false,
      "hide_identity": false
    },
    {
      "user_id": 456,
      "username": "PrivateUser",
      "name": "Jane Smith", 
      "latitude": -6.17,
      "longitude": 106.82,
      "last_update": 1756878146383,
      "is_active": true,
      "hide_location": true,
      "hide_identity": false
    }
  ],
  "total": 2,
  "hidden_from_public": 1,
  "last_updated": "2025-09-03T05:44:58Z"
}
```

## Heartbeat API

### Send Heartbeat with Privacy Settings
**URL**: `POST /api/spotters/heartbeat`  
**Auth**: Requires Sanctum token

**Request Body**:
```json
{
  "latitude": -6.175110,
  "longitude": 106.827153,
  "hide_location": false,
  "hide_identity": true
}
```

**Parameters**:
- `latitude` (required): GPS latitude (-90 to 90)
- `longitude` (required): GPS longitude (-180 to 180)  
- `hide_location` (optional): Hide completely from public map
- `hide_identity` (optional): Show as anonymous user

## Privacy Settings Combinations

| hide_location | hide_identity | Public Visibility | Admin Visibility |
|---------------|---------------|-------------------|------------------|
| `false` | `false` | Full visibility with username | Full details |
| `false` | `true` | Anonymous location marker | Full details |
| `true` | `false` | Completely hidden | Full details |
| `true` | `true` | Completely hidden | Full details |

## Implementation Details

### Database Storage
Privacy settings are stored in Redis with each heartbeat:
```go
type SpotterLocation struct {
    UserID           uint    `json:"user_id"`
    Username         string  `json:"username"`
    Name             string  `json:"name"`
    Latitude         float64 `json:"latitude"`
    Longitude        float64 `json:"longitude"`
    LastUpdate       int64   `json:"last_update"`
    IsActive         bool    `json:"is_active"`
    HideLocation     bool    `json:"hide_location"`
    HideIdentity     bool    `json:"hide_identity"`
}
```

### Response Types
**PublicSpotterLocation**: Used for public endpoint with privacy filtering
```go
type PublicSpotterLocation struct {
    UserID     *uint   `json:"user_id,omitempty"`
    Username   string  `json:"username"`
    Latitude   float64 `json:"latitude"`
    Longitude  float64 `json:"longitude"`
    LastUpdate int64   `json:"last_update"`
    IsActive   bool    `json:"is_active"`
}
```

**AdminSpottersResponse**: Used for admin endpoint with full visibility
```go
type AdminSpottersResponse struct {
    Spotters    []SpotterLocation `json:"spotters"`
    Total       int               `json:"total"`
    Hidden      int               `json:"hidden_from_public"`
    LastUpdated string            `json:"last_updated"`
}
```

### Cache Behavior
- Privacy settings are cached in Redis for 5 minutes
- Cache updates every 30 seconds
- Public endpoint filters cache based on privacy settings
- Admin endpoint returns unfiltered cache

## Testing Examples

### Test Identity Privacy (Heartbeat)
```bash
curl -X POST https://go-ltc.trainradar35.com/api/spotters/heartbeat \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"latitude": -6.175110, "longitude": 106.827153, "hide_identity": true}'
```

### Test Complete Hiding (Heartbeat)
```bash
curl -X POST https://go-ltc.trainradar35.com/api/spotters/heartbeat \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"latitude": -6.175110, "longitude": 106.827153, "hide_location": true}'
```

### Check Public Results (No Authentication)
```bash
# Anonymous access - always works
curl https://go-ltc.trainradar35.com/api/spotters/active
```

### Check Results with Regular User Token
```bash  
# Returns same filtered results as anonymous
curl https://go-ltc.trainradar35.com/api/spotters/active \
  -H "Authorization: Bearer REGULAR_USER_TOKEN"
```

### Check Admin Results (Same Endpoint, Admin Token)
```bash
# Returns full unfiltered admin data
curl https://go-ltc.trainradar35.com/api/spotters/active \
  -H "Authorization: Bearer ADMIN_TOKEN"
```

## Security Considerations

1. **Admin Role Validation**: Unified endpoint validates user has `role = 'admin'` in database
2. **Token Authentication**: Uses Laravel Sanctum token validation for admin detection
3. **Privacy Enforcement**: Strictly enforces privacy settings for non-admin users
4. **Data Separation**: Different response types prevent accidental data leakage
5. **Single Endpoint**: No separate admin URLs to discover or attack

## Mobile Integration

Mobile apps should:
1. **No Authentication Required**: Can call `/api/spotters/active` without tokens for public view
2. **Provide Privacy Controls**: Allow users to toggle privacy settings
3. **Persist Preferences**: Remember user's privacy choices
4. **Clear Messaging**: Explain what each privacy option does
5. **Default Settings**: Consider defaulting to some privacy protection
6. **Admin Enhancement**: Use admin tokens to get full data when available
7. **Graceful Degradation**: Work perfectly even without any authentication

The privacy system provides flexible control while maintaining the core functionality of the spotter location feature.