# Version Management Guide

This guide explains how to manage app versions without rebuilding the application.

## Current Setup

The app version is now managed via a JSON configuration file: **`version-config.json`**

```json
{
  "current_version": "1.2.0",
  "minimum_version": "1.1.0",
  "force_update": false,
  "update_message": "A new version is available with bug fixes and improvements!",
  "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases",
  "last_updated": "2025-08-08T10:00:00Z"
}
```

## How to Update Versions

### Method 1: Edit JSON File (Recommended)

**Step 1: Edit the JSON file on your server:**
```bash
# SSH to your server and edit the version config
nano version-config.json
```

**Step 2: Update the version numbers:**
```json
{
  "current_version": "1.3.0",
  "minimum_version": "1.2.0",
  "force_update": false,
  "update_message": "New features and security improvements!",
  "download_url": "https://github.com/manyunyu7/168railway-golang-ltc/releases"
}
```

**Step 3: Reload config (no restart needed!):**
```bash
# Reload config via API call
curl -X POST https://go-ltc.trainradar35.com/api/reload-version-config
```

**Alternative: Restart service (if reload fails):**
```bash
systemctl restart golang-live-tracking
```

### Method 2: Manual Code Update (Old Way)

If you prefer hardcoded values, edit `config/config.go`:
```go
CurrentVersion: getEnv("APP_CURRENT_VERSION", "1.3.0"), // ← Change default here
MinimumVersion: getEnv("APP_MINIMUM_VERSION", "1.2.0"), // ← Change default here
```

## Version Update Workflow

### 1. New App Release (Update Current Version)
```bash
# When you release app version 1.3.0
export APP_CURRENT_VERSION="1.3.0"
# Keep minimum version the same if older versions still work
# export APP_MINIMUM_VERSION="1.2.0"  # No change needed

# Restart service
systemctl restart golang-live-tracking
```

### 2. Deprecate Old Versions (Update Minimum Version)
```bash
# When you want to force users to update from versions older than 1.2.0
export APP_MINIMUM_VERSION="1.2.0"
export APP_CURRENT_VERSION="1.3.0"

# Restart service
systemctl restart golang-live-tracking
```

## No Rebuild Required! 🎉

**Benefits:**
- ✅ Update versions instantly via environment variables
- ✅ No need to rebuild or redeploy the Go application
- ✅ Can be automated via deployment scripts
- ✅ Rollback versions quickly if needed

## Testing Version Changes

### 1. Check Current Settings
```bash
curl https://go-ltc.trainradar35.com/api/app-version
```

### 2. Test Version Compatibility
```bash
# Test with old version (should fail if < minimum)
curl -X POST https://go-ltc.trainradar35.com/api/check-version \
  -H "Content-Type: application/json" \
  -d '{"version": "1.0.0"}'

# Test with current version (should pass)
curl -X POST https://go-ltc.trainradar35.com/api/check-version \
  -H "Content-Type: application/json" \
  -d '{"version": "1.3.0"}'
```

## Deployment Examples

### Docker
```bash
# Update via environment variables
docker run -e APP_CURRENT_VERSION="1.3.0" -e APP_MINIMUM_VERSION="1.2.0" golang-live-tracking
```

### Systemd Service
```ini
[Unit]
Description=Golang Live Tracking Service

[Service]
Environment=APP_CURRENT_VERSION=1.3.0
Environment=APP_MINIMUM_VERSION=1.2.0
ExecStart=/path/to/golang-live-tracking
Restart=always

[Install]
WantedBy=multi-user.target
```

### PM2 (Process Manager)
```json
{
  "name": "golang-live-tracking",
  "script": "./golang-live-tracking",
  "env": {
    "APP_CURRENT_VERSION": "1.3.0",
    "APP_MINIMUM_VERSION": "1.2.0"
  }
}
```

## Version Strategy Recommendations

### Conservative Approach
- Keep `minimum_version` 2-3 releases behind `current_version`
- Allows users time to update gradually

### Aggressive Approach  
- Set `minimum_version` = `current_version` for critical security updates
- Forces immediate updates

### Example Timeline
```
Week 1: Release 1.3.0
- current_version = "1.3.0" 
- minimum_version = "1.1.0" (still support 1.1.x and 1.2.x)

Week 4: Deprecate 1.1.x
- current_version = "1.3.0"
- minimum_version = "1.2.0" (force users off 1.1.x)

Week 8: Release 1.4.0
- current_version = "1.4.0"
- minimum_version = "1.2.0" (still support 1.2.x and 1.3.x)
```

## Troubleshooting

### Version Not Updating?
1. Check environment variables are set: `env | grep APP_`
2. Restart the service: `systemctl restart golang-live-tracking`
3. Verify via API: `curl .../api/app-version`

### Users Can't Access App?
- Check if their version is below `minimum_version`
- They'll get HTTP 426 (Upgrade Required) response
- Direct them to download latest version

## API Responses

### Supported Version
```json
{
  "success": true,
  "supported": true,
  "message": "You are using the latest version!"
}
```

### Update Available
```json
{
  "success": true,
  "supported": true,
  "update_available": true,
  "message": "A new version is available with bug fixes and improvements!"
}
```

### Force Update Required
```json
{
  "success": true,
  "supported": false,
  "force_update": true,
  "message": "Your app version is no longer supported. Please update to continue using the service."
}
```