# Version API Guide

This document describes the version check API endpoints that mobile apps can use to verify their compatibility and get update information.

## Overview

The version API provides endpoints to:
- Get current version information
- Validate client app versions
- Determine if updates are required or available

## Endpoints

### 1. Get App Version Info

**Endpoint:** `GET /api/app-version`

**Description:** Returns current version information and update details.

**Response:**
```json
{
  "current_version": "1.2.0",
  "minimum_version": "1.1.0", 
  "force_update": false,
  "update_message": "A new version is available with bug fixes and improvements!",
  "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases"
}
```

**Response Fields:**
- `current_version`: Latest available version
- `minimum_version`: Minimum supported version  
- `force_update`: Whether update is mandatory
- `update_message`: Message to show users
- `download_url`: Where to download updates

### 2. Check Version Compatibility

**Endpoint:** `POST /api/check-version`

**Description:** Validates a specific client version and provides detailed compatibility information.

**Request Body:**
```json
{
  "version": "1.1.5"
}
```

**Response (Supported Version):**
```json
{
  "success": true,
  "supported": true,
  "is_latest": false,
  "client_version": "1.1.5",
  "current_version": "1.2.0",
  "minimum_version": "1.1.0",
  "update_available": true,
  "message": "A new version is available with bug fixes and improvements!",
  "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases"
}
```

**Response (Unsupported Version):**
```json
{
  "success": true,
  "supported": false,
  "is_latest": false,
  "client_version": "1.0.5",
  "current_version": "1.2.0",
  "minimum_version": "1.1.0",
  "force_update": true,
  "message": "Your app version is no longer supported. Please update to continue using the service.",
  "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases"
}
```

**Response (Latest Version):**
```json
{
  "success": true,
  "supported": true,
  "is_latest": true,
  "client_version": "1.2.0",
  "current_version": "1.2.0",
  "minimum_version": "1.1.0",
  "message": "You are using the latest version!"
}
```

## Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | Whether the request was successful |
| `supported` | boolean | Whether the client version is supported |
| `is_latest` | boolean | Whether the client has the latest version |
| `client_version` | string | The version sent by the client |
| `current_version` | string | Latest available version |
| `minimum_version` | string | Minimum supported version |
| `force_update` | boolean | Whether update is mandatory (unsupported versions) |
| `update_available` | boolean | Whether a newer version is available |
| `message` | string | User-friendly message about version status |
| `download_url` | string | URL to download updates |

## Version Format

The API expects semantic versioning format: `MAJOR.MINOR.PATCH` (e.g., `1.2.0`)

- Versions are compared numerically
- Missing parts are treated as 0 (e.g., `1.2` = `1.2.0`)
- Supports 'v' prefix (e.g., `v1.2.0` = `1.2.0`)

## Integration Examples

### Flutter/Dart Example

```dart
import 'dart:convert';
import 'package:http/http.dart' as http;

Future<Map<String, dynamic>> checkVersion(String clientVersion) async {
  final response = await http.post(
    Uri.parse('https://go-ltc.trainradar35.com/api/check-version'),
    headers: {'Content-Type': 'application/json'},
    body: json.encode({'version': clientVersion}),
  );
  
  if (response.statusCode == 200) {
    return json.decode(response.body);
  } else {
    throw Exception('Failed to check version');
  }
}

// Usage
void main() async {
  try {
    final result = await checkVersion('1.1.5');
    
    if (!result['supported']) {
      // Force update required
      showForceUpdateDialog(result['message'], result['download_url']);
    } else if (result['update_available']) {
      // Optional update available
      showOptionalUpdateDialog(result['message'], result['download_url']);
    }
  } catch (e) {
    print('Version check failed: $e');
  }
}
```

### JavaScript/React Native Example

```javascript
const checkVersion = async (clientVersion) => {
  try {
    const response = await fetch('https://go-ltc.trainradar35.com/api/check-version', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ version: clientVersion }),
    });
    
    const result = await response.json();
    
    if (!result.supported) {
      // Force update required
      showForceUpdateAlert(result.message, result.download_url);
    } else if (result.update_available) {
      // Optional update available  
      showOptionalUpdateAlert(result.message, result.download_url);
    }
    
    return result;
  } catch (error) {
    console.error('Version check failed:', error);
  }
};
```

## Error Handling

### 400 Bad Request
```json
{
  "success": false,
  "error": "Invalid request format",
  "message": "Version parameter is required"
}
```

### 500 Internal Server Error
```json
{
  "success": false,
  "error": "Internal server error",
  "message": "Something went wrong"
}
```

## Configuration

To update version settings, modify the values in `/cmd/main.go`:

```go
// Version control endpoints
api.GET("/app-version", func(c *gin.Context) {
    c.JSON(200, gin.H{
        "current_version": "1.3.0",    // Update this for new releases
        "minimum_version": "1.2.0",    // Update this to deprecate old versions
        "force_update": false,
        "update_message": "New features and bug fixes available!",
        "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases",
    })
})
```

## Best Practices

1. **Check version on app startup** - Validate compatibility before allowing access to features
2. **Handle network failures gracefully** - Don't block app if version check fails
3. **Cache results temporarily** - Avoid checking on every app launch
4. **Provide clear messaging** - Explain why updates are needed
5. **Test version comparisons** - Ensure your version format matches expectations

## Production URLs

- **GET** `https://go-ltc.trainradar35.com/api/app-version`
- **POST** `https://go-ltc.trainradar35.com/api/check-version`

## Testing

You can test the endpoints using curl:

```bash
# Get version info
curl https://go-ltc.trainradar35.com/api/app-version

# Check specific version
curl -X POST https://go-ltc.trainradar35.com/api/check-version \
  -H "Content-Type: application/json" \
  -d '{"version": "1.1.5"}'
```