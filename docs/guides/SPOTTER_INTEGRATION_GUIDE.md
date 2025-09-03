# Spotter Location Integration Guide

## Quick Start

Get your spotter location feature up and running in 5 minutes.

### Step 1: Get Your Token
```javascript
// After user login, get Sanctum token
const token = "424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd";
```

### Step 2: Start Location Tracking
```javascript
// When user opens map
navigator.geolocation.getCurrentPosition(async (position) => {
  await fetch('https://go-ltc.trainradar35.com/api/spotters/heartbeat', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      latitude: position.coords.latitude,
      longitude: position.coords.longitude
    })
  });
});
```

### Step 3: Display Other Spotters
```javascript
// Get active spotters
const response = await fetch('https://go-ltc.trainradar35.com/api/spotters/active');
const data = await response.json();

// Add markers to map
data.spotters.forEach(spotter => {
  addSpotterMarker(spotter.latitude, spotter.longitude, spotter.name);
});
```

---

## Complete Implementation Examples

### React/Next.js Implementation

```jsx
// components/SpotterLocationService.jsx
import { useState, useEffect, useCallback } from 'react';

export const useSpotterLocation = (token) => {
  const [spotters, setSpotters] = useState([]);
  const [isTracking, setIsTracking] = useState(false);
  
  const sendHeartbeat = useCallback(async (latitude, longitude) => {
    if (!token) return;
    
    try {
      const response = await fetch('/api/spotters/heartbeat', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ latitude, longitude })
      });
      
      const result = await response.json();
      if (!result.success) {
        console.error('Heartbeat failed:', result.message);
      }
    } catch (error) {
      console.error('Heartbeat error:', error);
    }
  }, [token]);
  
  const fetchActiveSpotters = useCallback(async () => {
    try {
      const response = await fetch('/api/spotters/active');
      const data = await response.json();
      setSpotters(data.spotters || []);
    } catch (error) {
      console.error('Failed to fetch spotters:', error);
    }
  }, []);
  
  const startTracking = useCallback(() => {
    if (!navigator.geolocation) return;
    
    navigator.geolocation.getCurrentPosition(
      (position) => {
        const { latitude, longitude } = position.coords;
        sendHeartbeat(latitude, longitude);
        setIsTracking(true);
        
        // Send heartbeat every 2 minutes
        const interval = setInterval(() => {
          navigator.geolocation.getCurrentPosition(
            (pos) => sendHeartbeat(pos.coords.latitude, pos.coords.longitude)
          );
        }, 120000);
        
        return () => clearInterval(interval);
      },
      (error) => console.error('Geolocation error:', error)
    );
  }, [sendHeartbeat]);
  
  const stopTracking = useCallback(() => {
    setIsTracking(false);
  }, []);
  
  useEffect(() => {
    // Fetch spotters every 2 minutes
    const interval = setInterval(fetchActiveSpotters, 120000);
    fetchActiveSpotters(); // Initial fetch
    
    return () => clearInterval(interval);
  }, [fetchActiveSpotters]);
  
  return {
    spotters,
    isTracking,
    startTracking,
    stopTracking,
    refreshSpotters: fetchActiveSpotters
  };
};

// Usage in component
export default function MapComponent({ userToken }) {
  const { spotters, startTracking, stopTracking } = useSpotterLocation(userToken);
  
  useEffect(() => {
    // Start tracking when component mounts
    startTracking();
    
    // Stop tracking when component unmounts
    return () => stopTracking();
  }, [startTracking, stopTracking]);
  
  return (
    <div>
      <div>Active Spotters: {spotters.length}</div>
      {/* Your map implementation */}
      <Map>
        {spotters.map(spotter => (
          <SpotterMarker
            key={spotter.user_id}
            latitude={spotter.latitude}
            longitude={spotter.longitude}
            name={spotter.name}
            username={spotter.username}
          />
        ))}
      </Map>
    </div>
  );
}
```

### Vue.js Implementation

```vue
<!-- components/SpotterMap.vue -->
<template>
  <div>
    <div class="spotter-counter">
      Active Spotters: {{ spotters.length }}
    </div>
    
    <div id="map" ref="mapContainer"></div>
    
    <button @click="toggleTracking">
      {{ isTracking ? 'Stop Sharing Location' : 'Share My Location' }}
    </button>
  </div>
</template>

<script>
export default {
  name: 'SpotterMap',
  props: {
    sanctumToken: {
      type: String,
      required: true
    }
  },
  data() {
    return {
      spotters: [],
      isTracking: false,
      heartbeatInterval: null,
      spotterRefreshInterval: null,
      map: null,
      spotterMarkers: []
    };
  },
  
  async mounted() {
    await this.initializeMap();
    this.startSpotterRefresh();
  },
  
  beforeUnmount() {
    this.stopTracking();
    if (this.spotterRefreshInterval) {
      clearInterval(this.spotterRefreshInterval);
    }
  },
  
  methods: {
    async sendHeartbeat() {
      if (!navigator.geolocation) return;
      
      navigator.geolocation.getCurrentPosition(async (position) => {
        try {
          const response = await fetch('https://go-ltc.trainradar35.com/api/spotters/heartbeat', {
            method: 'POST',
            headers: {
              'Authorization': `Bearer ${this.sanctumToken}`,
              'Content-Type': 'application/json'
            },
            body: JSON.stringify({
              latitude: position.coords.latitude,
              longitude: position.coords.longitude
            })
          });
          
          const result = await response.json();
          if (!result.success) {
            console.error('Heartbeat failed:', result.message);
          }
        } catch (error) {
          console.error('Heartbeat error:', error);
        }
      });
    },
    
    async fetchSpotters() {
      try {
        const response = await fetch('https://go-ltc.trainradar35.com/api/spotters/active');
        const data = await response.json();
        this.spotters = data.spotters || [];
        this.updateMapMarkers();
      } catch (error) {
        console.error('Failed to fetch spotters:', error);
      }
    },
    
    toggleTracking() {
      if (this.isTracking) {
        this.stopTracking();
      } else {
        this.startTracking();
      }
    },
    
    startTracking() {
      this.sendHeartbeat(); // Initial heartbeat
      
      // Send heartbeat every 2 minutes
      this.heartbeatInterval = setInterval(() => {
        this.sendHeartbeat();
      }, 120000);
      
      this.isTracking = true;
    },
    
    stopTracking() {
      if (this.heartbeatInterval) {
        clearInterval(this.heartbeatInterval);
        this.heartbeatInterval = null;
      }
      this.isTracking = false;
    },
    
    startSpotterRefresh() {
      this.fetchSpotters(); // Initial fetch
      
      // Refresh every 2 minutes
      this.spotterRefreshInterval = setInterval(() => {
        this.fetchSpotters();
      }, 120000);
    },
    
    updateMapMarkers() {
      // Clear existing markers
      this.spotterMarkers.forEach(marker => marker.remove());
      this.spotterMarkers = [];
      
      // Add new markers
      this.spotters.forEach(spotter => {
        const marker = new google.maps.Marker({
          position: { lat: spotter.latitude, lng: spotter.longitude },
          map: this.map,
          title: `${spotter.name} (${spotter.username})`,
          icon: {
            url: '/icons/spotter-marker.png',
            scaledSize: new google.maps.Size(20, 20)
          }
        });
        
        this.spotterMarkers.push(marker);
      });
    }
  }
};
</script>
```

### PHP Laravel Frontend Integration

```php
<?php
// app/Http/Controllers/MapController.php

namespace App\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Support\Facades\Http;

class MapController extends Controller
{
    public function showMap()
    {
        $user = auth()->user();
        $token = $user->createToken('spotter-access')->plainTextToken;
        
        return view('map.index', [
            'spotterToken' => $token,
            'user' => $user
        ]);
    }
    
    public function getActiveSpotters()
    {
        try {
            $response = Http::get('https://go-ltc.trainradar35.com/api/spotters/active');
            return $response->json();
        } catch (Exception $e) {
            return response()->json(['error' => 'Failed to fetch spotters'], 500);
        }
    }
}
```

```blade
{{-- resources/views/map/index.blade.php --}}
@extends('layouts.app')

@section('content')
<div id="map-container">
    <div class="spotter-controls">
        <button id="toggle-sharing" class="btn btn-primary">
            Share My Location
        </button>
        <div id="spotter-count">Active Spotters: 0</div>
    </div>
    
    <div id="map" style="height: 500px;"></div>
</div>

<script>
document.addEventListener('DOMContentLoaded', function() {
    const token = '{{ $spotterToken }}';
    let map;
    let isSharing = false;
    let heartbeatInterval = null;
    let spotterMarkers = [];
    
    // Initialize map
    function initMap() {
        map = new google.maps.Map(document.getElementById('map'), {
            zoom: 10,
            center: { lat: -6.200000, lng: 106.816666 }
        });
        
        loadActiveSpotters();
        setInterval(loadActiveSpotters, 120000); // Refresh every 2 minutes
    }
    
    // Load active spotters
    async function loadActiveSpotters() {
        try {
            const response = await fetch('/map/active-spotters');
            const data = await response.json();
            
            document.getElementById('spotter-count').textContent = 
                `Active Spotters: ${data.total || 0}`;
            
            updateSpotterMarkers(data.spotters || []);
        } catch (error) {
            console.error('Failed to load spotters:', error);
        }
    }
    
    // Send location heartbeat
    async function sendHeartbeat() {
        if (!navigator.geolocation) return;
        
        navigator.geolocation.getCurrentPosition(async (position) => {
            try {
                await fetch('https://go-ltc.trainradar35.com/api/spotters/heartbeat', {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${token}`,
                        'Content-Type': 'application/json',
                        'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]').content
                    },
                    body: JSON.stringify({
                        latitude: position.coords.latitude,
                        longitude: position.coords.longitude
                    })
                });
            } catch (error) {
                console.error('Heartbeat failed:', error);
            }
        });
    }
    
    // Toggle location sharing
    document.getElementById('toggle-sharing').addEventListener('click', function() {
        if (isSharing) {
            // Stop sharing
            if (heartbeatInterval) {
                clearInterval(heartbeatInterval);
                heartbeatInterval = null;
            }
            this.textContent = 'Share My Location';
            isSharing = false;
        } else {
            // Start sharing
            sendHeartbeat(); // Initial heartbeat
            heartbeatInterval = setInterval(sendHeartbeat, 120000); // Every 2 minutes
            this.textContent = 'Stop Sharing';
            isSharing = true;
        }
    });
    
    // Update map markers
    function updateSpotterMarkers(spotters) {
        // Clear existing markers
        spotterMarkers.forEach(marker => marker.setMap(null));
        spotterMarkers = [];
        
        // Add new markers
        spotters.forEach(spotter => {
            const marker = new google.maps.Marker({
                position: { lat: spotter.latitude, lng: spotter.longitude },
                map: map,
                title: `${spotter.name} (${spotter.username})`,
                icon: {
                    url: '/images/spotter-icon.png',
                    scaledSize: new google.maps.Size(20, 20)
                }
            });
            
            spotterMarkers.push(marker);
        });
    }
    
    // Initialize map when page loads
    initMap();
});
</script>
@endsection
```

### Flutter/Dart Mobile Implementation

```dart
// lib/services/spotter_location_service.dart
import 'dart:async';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:geolocator/geolocator.dart';

class SpotterLocationService {
  static const String baseUrl = 'https://go-ltc.trainradar35.com/api/spotters';
  
  final String token;
  Timer? _heartbeatTimer;
  Timer? _spotterRefreshTimer;
  
  SpotterLocationService({required this.token});
  
  // Send heartbeat with current location
  Future<bool> sendHeartbeat({required double latitude, required double longitude}) async {
    try {
      final response = await http.post(
        Uri.parse('$baseUrl/heartbeat'),
        headers: {
          'Authorization': 'Bearer $token',
          'Content-Type': 'application/json',
        },
        body: jsonEncode({
          'latitude': latitude,
          'longitude': longitude,
        }),
      );
      
      final data = jsonDecode(response.body);
      return data['success'] ?? false;
    } catch (e) {
      print('Heartbeat error: $e');
      return false;
    }
  }
  
  // Get active spotters
  Future<List<SpotterLocation>> getActiveSpotters() async {
    try {
      final response = await http.get(Uri.parse('$baseUrl/active'));
      final data = jsonDecode(response.body);
      
      if (data['spotters'] != null) {
        return (data['spotters'] as List)
            .map((json) => SpotterLocation.fromJson(json))
            .toList();
      }
      return [];
    } catch (e) {
      print('Failed to fetch spotters: $e');
      return [];
    }
  }
  
  // Start location sharing
  Future<void> startLocationSharing() async {
    final permission = await Geolocator.requestPermission();
    if (permission == LocationPermission.denied) {
      throw Exception('Location permission denied');
    }
    
    // Send initial heartbeat
    final position = await Geolocator.getCurrentPosition();
    await sendHeartbeat(
      latitude: position.latitude,
      longitude: position.longitude,
    );
    
    // Start periodic heartbeats every 2 minutes
    _heartbeatTimer = Timer.periodic(const Duration(minutes: 2), (timer) async {
      try {
        final position = await Geolocator.getCurrentPosition();
        await sendHeartbeat(
          latitude: position.latitude,
          longitude: position.longitude,
        );
      } catch (e) {
        print('Heartbeat failed: $e');
      }
    });
  }
  
  // Stop location sharing
  void stopLocationSharing() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
  }
  
  // Start spotter refresh
  void startSpotterRefresh(Function(List<SpotterLocation>) onUpdate) {
    // Initial fetch
    getActiveSpotters().then(onUpdate);
    
    // Periodic refresh every 2 minutes
    _spotterRefreshTimer = Timer.periodic(const Duration(minutes: 2), (timer) {
      getActiveSpotters().then(onUpdate);
    });
  }
  
  // Stop spotter refresh
  void stopSpotterRefresh() {
    _spotterRefreshTimer?.cancel();
    _spotterRefreshTimer = null;
  }
  
  void dispose() {
    stopLocationSharing();
    stopSpotterRefresh();
  }
}

// lib/models/spotter_location.dart
class SpotterLocation {
  final int userId;
  final String username;
  final String name;
  final double latitude;
  final double longitude;
  final int lastUpdate;
  final bool isActive;
  
  SpotterLocation({
    required this.userId,
    required this.username,
    required this.name,
    required this.latitude,
    required this.longitude,
    required this.lastUpdate,
    required this.isActive,
  });
  
  factory SpotterLocation.fromJson(Map<String, dynamic> json) {
    return SpotterLocation(
      userId: json['user_id'],
      username: json['username'],
      name: json['name'],
      latitude: json['latitude'].toDouble(),
      longitude: json['longitude'].toDouble(),
      lastUpdate: json['last_update'],
      isActive: json['is_active'],
    );
  }
}

// lib/screens/map_screen.dart
class MapScreen extends StatefulWidget {
  final String sanctumToken;
  
  const MapScreen({Key? key, required this.sanctumToken}) : super(key: key);
  
  @override
  _MapScreenState createState() => _MapScreenState();
}

class _MapScreenState extends State<MapScreen> {
  late SpotterLocationService _spotterService;
  List<SpotterLocation> _spotters = [];
  bool _isSharingLocation = false;
  
  @override
  void initState() {
    super.initState();
    _spotterService = SpotterLocationService(token: widget.sanctumToken);
    _spotterService.startSpotterRefresh(_updateSpotters);
  }
  
  @override
  void dispose() {
    _spotterService.dispose();
    super.dispose();
  }
  
  void _updateSpotters(List<SpotterLocation> spotters) {
    setState(() {
      _spotters = spotters;
    });
  }
  
  void _toggleLocationSharing() async {
    if (_isSharingLocation) {
      _spotterService.stopLocationSharing();
      setState(() {
        _isSharingLocation = false;
      });
    } else {
      try {
        await _spotterService.startLocationSharing();
        setState(() {
          _isSharingLocation = true;
        });
      } catch (e) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Failed to start location sharing: $e')),
        );
      }
    }
  }
  
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text('Train Map'),
        actions: [
          Text('Spotters: ${_spotters.length}'),
          SizedBox(width: 16),
        ],
      ),
      body: Column(
        children: [
          Container(
            padding: EdgeInsets.all(16),
            child: ElevatedButton(
              onPressed: _toggleLocationSharing,
              child: Text(_isSharingLocation ? 'Stop Sharing Location' : 'Share My Location'),
            ),
          ),
          Expanded(
            child: GoogleMap(
              // Your map implementation
              markers: _spotters.map((spotter) => Marker(
                markerId: MarkerId(spotter.userId.toString()),
                position: LatLng(spotter.latitude, spotter.longitude),
                infoWindow: InfoWindow(
                  title: spotter.name,
                  snippet: spotter.username,
                ),
              )).toSet(),
            ),
          ),
        ],
      ),
    );
  }
}
```

---

## Best Practices

### 1. User Experience
- **Ask for permission** before starting location tracking
- **Provide clear feedback** when location sharing is active
- **Allow users to opt-out** easily
- **Show spotter count** to indicate activity

### 2. Performance Optimization
- **Cache responses** locally for 30 seconds
- **Batch API calls** to reduce server load  
- **Use appropriate intervals** (2 minutes recommended)
- **Handle offline scenarios** gracefully

### 3. Error Handling
```javascript
// Robust error handling example
async function sendHeartbeatWithRetry(latitude, longitude, maxRetries = 3) {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch('/api/spotters/heartbeat', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ latitude, longitude })
      });
      
      if (response.status === 401) {
        // Token expired - redirect to login
        window.location.href = '/login';
        return;
      }
      
      if (response.ok) {
        return await response.json();
      }
      
      throw new Error(`HTTP ${response.status}`);
    } catch (error) {
      if (attempt === maxRetries) {
        console.error('Max retries reached:', error);
        throw error;
      }
      
      // Exponential backoff
      await new Promise(resolve => setTimeout(resolve, Math.pow(2, attempt) * 1000));
    }
  }
}
```

### 4. Privacy & Security
- **Respect user privacy** settings
- **Store tokens securely** (not in localStorage for sensitive apps)
- **Validate coordinates** before sending
- **Handle location permission** properly

---

## Testing & Debugging

### Test Token
Use this token for development testing:
```
424|nVD9mNiWqrizuSDATBT2TQxBQcOz7SlmFIEPNm5I925e22cd
```

### Debug Checklist
- ✅ Token format correct (ID|TOKEN)
- ✅ Coordinates within valid ranges
- ✅ Authorization header included
- ✅ Content-Type is application/json
- ✅ HTTPS used for production calls

### Common Issues
1. **401 Unauthorized**: Check token format and expiration
2. **400 Bad Request**: Validate latitude/longitude values
3. **CORS errors**: Ensure proper headers for web requests
4. **Rate limiting**: Don't exceed 30 heartbeats per hour

---

## Migration from Other Solutions

### From WebSocket to REST API
```javascript
// Before: WebSocket
const ws = new WebSocket('wss://example.com/spotters');
ws.onmessage = (event) => {
  const spotters = JSON.parse(event.data);
  updateMap(spotters);
};

// After: REST API with polling
async function pollSpotters() {
  const response = await fetch('/api/spotters/active');
  const data = await response.json();
  updateMap(data.spotters);
}
setInterval(pollSpotters, 120000);
```

### From Polling to Optimized Caching
```javascript
// Before: Frequent polling
setInterval(() => fetch('/api/spotters/active'), 10000); // Every 10 seconds

// After: Optimized with cache awareness
let lastUpdated = null;
async function fetchSpottersOptimized() {
  const response = await fetch('/api/spotters/active');
  const data = await response.json();
  
  // Only update UI if data actually changed
  if (data.last_updated !== lastUpdated) {
    lastUpdated = data.last_updated;
    updateMap(data.spotters);
  }
}
setInterval(fetchSpottersOptimized, 120000); // Every 2 minutes
```

---

This integration guide provides everything you need to implement the spotter location feature in your application. Choose the example that matches your technology stack and customize as needed.

*For additional support, check the main API documentation or create an issue on GitHub.*