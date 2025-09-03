# Authentication Guide for Spotter Location API

## Overview

The Spotter Location API uses **Laravel Sanctum tokens** for authentication. This guide covers everything you need to know about obtaining, using, and managing authentication tokens.

## Token Format

Sanctum tokens follow this format:
```
{token_id}|{token_string}
```

**Example:**
```
424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd
```

Where:
- `424` is the token ID in the database
- `nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd` is the token string

## How Authentication Works

### 1. Token Validation Process

When you send a request with a Bearer token:

```http
Authorization: Bearer 424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd
```

The server performs these steps:

1. **Splits the token** at the `|` character
2. **Hashes the token string** using SHA256
3. **Queries the database** for matching ID and hash
4. **Validates expiration** if set
5. **Updates last_used_at** timestamp
6. **Retrieves user information**

### 2. Database Structure

Tokens are stored in the `personal_access_tokens` table:

```sql
CREATE TABLE personal_access_tokens (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tokenable_type VARCHAR(255) NOT NULL,
    tokenable_id BIGINT UNSIGNED NOT NULL,
    name VARCHAR(255) NOT NULL,
    token VARCHAR(64) NOT NULL,  -- SHA256 hash of token string
    abilities TEXT,
    last_used_at TIMESTAMP NULL,
    expires_at TIMESTAMP NULL,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL
);
```

---

## Obtaining Tokens

### For Mobile Applications

#### 1. Login via Mobile API
```bash
curl -X POST https://168railway.com/api/mobile/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password"
  }'
```

#### Response:
```json
{
  "success": true,
  "token": "424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd",
  "user": {
    "id": 12,
    "name": "Henry Augusta",
    "email": "user@example.com"
  }
}
```

#### 2. Use Token for Spotter API
```javascript
const token = "424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd";

fetch('https://go-ltc.trainradar35.com/api/spotters/heartbeat', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    latitude: -6.200000,
    longitude: 106.816666
  })
});
```

### For Web Applications

#### Method 1: Laravel Session + Token Generation

**Step 1: User logs in via Laravel web interface**
```php
// routes/web.php
Route::post('/login', [AuthController::class, 'login']);
```

**Step 2: Generate API token after login**
```php
// In your controller
public function generateApiToken(Request $request)
{
    $user = auth()->user();
    
    // Create a new token for spotter API
    $token = $user->createToken('spotter-access', ['spotter:read', 'spotter:write']);
    
    return response()->json([
        'token' => $token->plainTextToken,
        'expires_at' => null // or set expiration
    ]);
}
```

**Step 3: Use token in JavaScript**
```javascript
// Get token from your API
const response = await fetch('/api/generate-token');
const data = await response.json();
const token = data.token;

// Use for spotter API
fetch('https://go-ltc.trainradar35.com/api/spotters/heartbeat', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({ latitude: -6.2, longitude: 106.8 })
});
```

#### Method 2: Direct Token Creation in Laravel

```php
// In your Laravel application
use Laravel\Sanctum\HasApiTokens;

class User extends Authenticatable
{
    use HasApiTokens;
    
    // Create token with specific abilities
    public function createSpotterToken()
    {
        return $this->createToken('spotter-access', [
            'spotter:heartbeat',
            'spotter:read'
        ]);
    }
}

// Usage
$user = User::find(1);
$token = $user->createSpotterToken();
echo $token->plainTextToken; // Use this for API calls
```

#### Method 3: User Dashboard Integration

Create a user dashboard where users can generate their own tokens:

```php
// app/Http/Controllers/UserTokenController.php
class UserTokenController extends Controller
{
    public function index()
    {
        $user = auth()->user();
        $tokens = $user->tokens()->where('name', 'spotter-access')->get();
        
        return view('user.tokens', compact('tokens'));
    }
    
    public function create(Request $request)
    {
        $user = auth()->user();
        
        // Revoke existing spotter tokens
        $user->tokens()->where('name', 'spotter-access')->delete();
        
        // Create new token
        $token = $user->createToken('spotter-access', [
            'spotter:heartbeat',
            'spotter:read'
        ]);
        
        return back()->with('token', $token->plainTextToken);
    }
    
    public function revoke($tokenId)
    {
        auth()->user()->tokens()->where('id', $tokenId)->delete();
        
        return back()->with('message', 'Token revoked successfully');
    }
}
```

```blade
{{-- resources/views/user/tokens.blade.php --}}
@extends('layouts.app')

@section('content')
<div class="container">
    <div class="card">
        <div class="card-header">
            <h4>API Tokens for Spotter Location</h4>
        </div>
        <div class="card-body">
            @if(session('token'))
                <div class="alert alert-success">
                    <strong>New Token Created:</strong><br>
                    <code>{{ session('token') }}</code>
                    <br><small>Save this token - you won't see it again!</small>
                </div>
            @endif
            
            <form method="POST" action="{{ route('user.tokens.create') }}">
                @csrf
                <button type="submit" class="btn btn-primary">
                    Generate New Spotter Token
                </button>
            </form>
            
            <hr>
            
            <h5>Active Tokens</h5>
            <div class="table-responsive">
                <table class="table">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Last Used</th>
                            <th>Created</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        @forelse($tokens as $token)
                        <tr>
                            <td>{{ $token->name }}</td>
                            <td>{{ $token->last_used_at ? $token->last_used_at->diffForHumans() : 'Never' }}</td>
                            <td>{{ $token->created_at->diffForHumans() }}</td>
                            <td>
                                <form method="POST" action="{{ route('user.tokens.revoke', $token->id) }}">
                                    @csrf
                                    @method('DELETE')
                                    <button type="submit" class="btn btn-sm btn-danger">Revoke</button>
                                </form>
                            </td>
                        </tr>
                        @empty
                        <tr>
                            <td colspan="4" class="text-center">No active tokens</td>
                        </tr>
                        @endforelse
                    </tbody>
                </table>
            </div>
        </div>
    </div>
</div>
@endsection
```

---

## Token Security

### Best Practices

1. **Store Tokens Securely**
   ```javascript
   // ❌ Don't store in localStorage for sensitive apps
   localStorage.setItem('token', token);
   
   // ✅ Better: Use secure HTTP-only cookies
   // Or store in memory for single session
   ```

2. **Use HTTPS Only**
   ```javascript
   // ❌ Never use HTTP for production
   fetch('http://api.example.com/spotters/heartbeat', {
     headers: { 'Authorization': `Bearer ${token}` }
   });
   
   // ✅ Always use HTTPS
   fetch('https://api.example.com/spotters/heartbeat', {
     headers: { 'Authorization': `Bearer ${token}` }
   });
   ```

3. **Handle Token Expiration**
   ```javascript
   async function apiCall(url, options) {
     const response = await fetch(url, options);
     
     if (response.status === 401) {
       // Token expired or invalid
       localStorage.removeItem('token');
       window.location.href = '/login';
       return;
     }
     
     return response.json();
   }
   ```

4. **Rotate Tokens Regularly**
   ```php
   // Rotate token every 30 days
   public function rotateToken(User $user)
   {
       // Revoke old tokens
       $user->tokens()->where('name', 'spotter-access')->delete();
       
       // Create new token
       return $user->createToken('spotter-access', ['spotter:*']);
   }
   ```

### Token Abilities

You can restrict token permissions using abilities:

```php
// Create token with specific abilities
$token = $user->createToken('spotter-limited', [
    'spotter:heartbeat',  // Can send heartbeats
    'spotter:read'        // Can read active spotters
]);

// Check abilities in middleware
if (!$request->user()->tokenCan('spotter:heartbeat')) {
    return response()->json(['error' => 'Insufficient permissions'], 403);
}
```

### Token Expiration

```php
// Set token expiration
$token = $user->createToken('spotter-access', ['*'], now()->addDays(30));

// Or set default expiration in config
// config/sanctum.php
'expiration' => 60 * 24 * 30, // 30 days in minutes
```

---

## Debugging Authentication Issues

### Common Error Messages

#### 1. "Authentication required"
```json
{
  "success": false,
  "message": "Authentication required - please provide Sanctum token"
}
```
**Solution**: Add `Authorization: Bearer {token}` header

#### 2. "Invalid authorization header format"
```json
{
  "success": false,
  "message": "Invalid authorization header format"
}
```
**Solution**: Ensure format is `Bearer {token_id}|{token_string}`

#### 3. "Invalid token format"
```json
{
  "success": false,
  "message": "Invalid token format"
}
```
**Solution**: Token must contain `|` separator

#### 4. "Invalid or expired token"
```json
{
  "success": false,
  "message": "Invalid or expired token"
}
```
**Solutions**:
- Check token exists in database
- Verify token hasn't expired
- Ensure token string matches stored hash

#### 5. "Token has expired"
```json
{
  "success": false,
  "message": "Token has expired"
}
```
**Solution**: Generate a new token

### Debug Mode

The server includes debug logging for authentication. Check server logs for:

```
DEBUG: Plain text token: 424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd
DEBUG: Token ID: 424
DEBUG: Token string (after |): nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd
DEBUG: Hashed token: a1b2c3d4e5f6...
DEBUG: Database query error: record not found
```

### Testing Authentication

Test your token with curl:

```bash
# Test valid token
curl -X POST \
  -H "Authorization: Bearer 424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd" \
  -H "Content-Type: application/json" \
  -d '{"latitude": -6.2, "longitude": 106.8}' \
  https://go-ltc.trainradar35.com/api/spotters/heartbeat

# Expected response
{"success":true,"message":"Spotter location updated"}
```

---

## Rate Limiting

### Current Limits
- **Heartbeat endpoint**: 30 requests per hour per token
- **Active spotters endpoint**: No limit (public, cached)

### Implementing Client-Side Rate Limiting

```javascript
class RateLimitedSpotterService {
  constructor(token) {
    this.token = token;
    this.lastHeartbeat = 0;
    this.minInterval = 120000; // 2 minutes
  }
  
  async sendHeartbeat(latitude, longitude) {
    const now = Date.now();
    
    if (now - this.lastHeartbeat < this.minInterval) {
      console.log('Heartbeat rate limited');
      return;
    }
    
    try {
      const response = await fetch('/api/spotters/heartbeat', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${this.token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ latitude, longitude })
      });
      
      if (response.ok) {
        this.lastHeartbeat = now;
      }
      
      return response.json();
    } catch (error) {
      console.error('Heartbeat failed:', error);
    }
  }
}
```

---

## Production Considerations

### 1. Token Storage in Production

**Mobile Apps:**
- Use secure keychain/keystore
- Don't store in app preferences
- Consider token refresh flow

**Web Apps:**
- HTTP-only secure cookies (if possible)
- Memory storage for session-only tokens
- Avoid localStorage for sensitive tokens

### 2. Token Monitoring

```php
// Monitor token usage
class TokenUsageMonitor
{
    public static function logTokenUsage($token, $endpoint)
    {
        Log::info('Token usage', [
            'token_id' => $token->id,
            'user_id' => $token->tokenable_id,
            'endpoint' => $endpoint,
            'ip' => request()->ip(),
            'user_agent' => request()->userAgent(),
            'timestamp' => now()
        ]);
    }
}
```

### 3. Token Cleanup

```php
// Artisan command to clean old tokens
class CleanupTokens extends Command
{
    protected $signature = 'tokens:cleanup';
    protected $description = 'Remove expired and unused tokens';
    
    public function handle()
    {
        // Remove tokens not used in 30 days
        $deleted = PersonalAccessToken::where('last_used_at', '<', now()->subDays(30))
            ->orWhere('created_at', '<', now()->subDays(30))
            ->where('last_used_at', null)
            ->delete();
            
        $this->info("Cleaned up {$deleted} unused tokens");
        
        // Remove expired tokens
        $expired = PersonalAccessToken::where('expires_at', '<', now())
            ->delete();
            
        $this->info("Cleaned up {$expired} expired tokens");
    }
}
```

---

This authentication guide provides comprehensive coverage of token management for the Spotter Location API. For additional security considerations, consult your security team and follow Laravel Sanctum best practices.