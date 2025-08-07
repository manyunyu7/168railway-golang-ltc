# 🚂 Trip Saving Documentation

## Overview

When users complete their live tracking session, they can choose to save their journey as a **Trip** record in the database. This creates a permanent record of their travel with detailed tracking data.

---

## 🛤️ How Trip Saving Works

### **1. User Request**

#### **Basic Stop (No Trip Saving)**
```http
POST /api/mobile/live-tracking/stop
Authorization: Bearer {token}
Content-Type: application/json

{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "save_trip": false
}
```

#### **Stop with Mobile-Calculated Trip Statistics** ⭐ **PREFERRED**
```http
POST /api/mobile/live-tracking/stop
Authorization: Bearer {token}
Content-Type: application/json

{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "save_trip": true,
  "trip_summary": {
    "total_distance_km": 125.8,
    "max_speed_kmh": 82.5,
    "avg_speed_kmh": 45.2,
    "duration_seconds": 10800,
    "max_elevation_m": 245,
    "min_elevation_m": 95,
    "elevation_gain_m": 150,
    "max_speed_location": {
      "lat": -6.5234,
      "lng": 106.8456
    },
    "max_elevation_location": {
      "lat": -6.3123,
      "lng": 106.9234
    }
  }
}
```

#### **Fallback: Stop with Server Calculation**
```http
POST /api/mobile/live-tracking/stop
Authorization: Bearer {token}
Content-Type: application/json

{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "save_trip": true
}
```

### **2. Server Process**

#### **With Mobile Statistics (Preferred)** ⚡
1. ✅ **Validate Session** - Check active session in `live_tracking_sessions` table
2. ✅ **Extract Tracking Data** - Get GPS points from S3 for JSON storage only
3. ✅ **Use Mobile Stats** - Accept pre-calculated statistics from mobile app
4. ✅ **Save to Database** - Create record in `trips` table with mobile stats
5. ✅ **Return Trip ID** - Confirm successful save

#### **Fallback Mode (Server Calculation)** 🔄
1. ✅ **Validate Session** - Check active session in `live_tracking_sessions` table
2. ✅ **Extract Tracking Data** - Get all GPS points from S3 train file
3. ✅ **Calculate Statistics** - Server calculates distance, speed, elevation using Haversine formula
4. ✅ **Save to Database** - Create record in `trips` table with server stats
5. ✅ **Return Trip ID** - Confirm successful save

### **3. API Response**
```json
{
  "success": true,
  "message": "Mobile tracking session stopped successfully",
  "trip_saved": true,
  "trip_id": 1234
}
```

---

## 📊 Database Schema

### **`trips` Table Structure**
```sql
CREATE TABLE trips (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    session_id VARCHAR(255),
    user_id BIGINT,
    user_type VARCHAR(50) DEFAULT 'authenticated',
    train_id BIGINT,
    train_name VARCHAR(255),
    train_number VARCHAR(255),
    total_distance_km DECIMAL(8,3) DEFAULT 0,
    max_speed_kmh DECIMAL(6,2) DEFAULT 0,
    avg_speed_kmh DECIMAL(6,2) DEFAULT 0,
    max_elevation_m INT DEFAULT 0,
    min_elevation_m INT DEFAULT 0,
    elevation_gain_m INT DEFAULT 0,
    duration_seconds INT,
    start_latitude DECIMAL(10,8),
    start_longitude DECIMAL(11,8),
    end_latitude DECIMAL(10,8),
    end_longitude DECIMAL(11,8),
    max_speed_lat DECIMAL(10,8) NULL,
    max_speed_lng DECIMAL(11,8) NULL,
    max_elevation_lat DECIMAL(10,8) NULL,
    max_elevation_lng DECIMAL(11,8) NULL,
    from_station_id BIGINT NULL,
    from_station_name VARCHAR(255) NULL,
    to_station_id BIGINT NULL,
    to_station_name VARCHAR(255) NULL,
    tracking_data LONGTEXT,           -- JSON array of GPS points
    route_coordinates LONGTEXT,       -- Simplified coordinates for maps
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

---

## 📍 Data Structure

### **Tracking Data JSON** (`tracking_data` field)
Complete GPS tracking points with metadata:
```json
[
  {
    "userID": 123,
    "userType": "authenticated",
    "clientType": "mobile",
    "lat": -6.208763,
    "lng": 106.845599,
    "timestamp": 1691427600000,
    "accuracy": 5.0,
    "speed": 45.5,
    "heading": 180.5,
    "altitude": 125.0,
    "sessionID": "550e8400-e29b-41d4-a716-446655440000",
    "status": "active"
  },
  {
    "userID": 123,
    "userType": "authenticated", 
    "clientType": "mobile",
    "lat": -6.209123,
    "lng": 106.846234,
    "timestamp": 1691427630000,
    "accuracy": 4.8,
    "speed": 48.2,
    "heading": 175.3,
    "altitude": 128.5,
    "sessionID": "550e8400-e29b-41d4-a716-446655440000",
    "status": "active"
  }
]
```

### **Route Coordinates JSON** (`route_coordinates` field)
Simplified coordinates for map display:
```json
[
  {
    "lat": -6.208763,
    "lng": 106.845599,
    "timestamp": 1691427600000
  },
  {
    "lat": -6.209123,
    "lng": 106.846234,
    "timestamp": 1691427630000
  }
]
```

---

## 🎯 Sample Data

### **Example Trip Record**
*Based on actual database record with longest distance*

```json
{
  "id": 1234,
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": 123,
  "user_type": "authenticated",
  "train_id": 501,
  "train_name": "KA Argo Bromo Anggrek",
  "train_number": "KA501",
  "total_distance_km": 125.750,
  "max_speed_kmh": 78.50,
  "avg_speed_kmh": 42.30,
  "duration_seconds": 10800,
  "start_latitude": -6.208763,
  "start_longitude": 106.845599,
  "end_latitude": -7.325412,
  "end_longitude": 112.781345,
  "from_station_name": "Jakarta Gambir",
  "to_station_name": "Surabaya Gubeng",
  "started_at": "2025-08-07T10:00:00Z",
  "completed_at": "2025-08-07T13:00:00Z",
  "tracking_data": "[{...}]", // Complete GPS points
  "route_coordinates": "[{...}]" // Map coordinates
}
```

---

## 🔧 Implementation Details

### **Trip Saving Process**
```go
func (h *SimpleLiveTrackingHandler) saveUserTrip(session models.LiveTrackingSession, userID uint) *uint {
    // 1. Extract tracking data from S3
    trainData, err := h.s3.GetTrainData(session.FilePath)
    
    // 2. Filter user's GPS points
    var userTrackingData []models.Passenger
    for _, passenger := range trainData.Passengers {
        if passenger.UserID == userID {
            userTrackingData = append(userTrackingData, passenger)
        }
    }
    
    // 3. Calculate trip statistics
    startPoint := userTrackingData[0]
    endPoint := userTrackingData[len(userTrackingData)-1]
    durationSeconds := int((endPoint.Timestamp - startPoint.Timestamp) / 1000)
    
    // 4. Save to database
    trip := models.Trip{
        SessionID:        session.SessionID,
        UserID:           &userID,
        TrainID:          session.TrainID,
        TrainNumber:      session.TrainNumber,
        DurationSeconds:  durationSeconds,
        StartLatitude:    startPoint.Lat,
        StartLongitude:   startPoint.Lng,
        EndLatitude:      endPoint.Lat,
        EndLongitude:     endPoint.Lng,
        TrackingData:     string(trackingDataJSON),
        RouteCoordinates: string(routeCoordsJSON),
        StartedAt:        session.StartedAt,
        CompletedAt:      time.Now(),
    }
    
    h.db.Create(&trip)
    return &trip.ID
}
```

---

## 🚀 Usage Examples

### **1. Mobile App Trip Calculation** ⭐ **RECOMMENDED**
```javascript
// JavaScript/Flutter example - Calculate stats on mobile during tracking
class TripTracker {
    constructor() {
        this.startTime = null;
        this.totalDistance = 0;
        this.maxSpeed = 0;
        this.speeds = [];
        this.maxElevation = null;
        this.minElevation = null;
        this.maxSpeedLocation = null;
        this.maxElevationLocation = null;
    }
    
    // Called on each GPS update during tracking
    updatePosition(lat, lng, speed, altitude, accuracy) {
        if (this.lastPosition) {
            // Calculate distance increment using Haversine formula
            const distance = this.calculateDistance(
                this.lastPosition.lat, this.lastPosition.lng, lat, lng
            );
            this.totalDistance += distance;
        }
        
        // Track speed statistics
        if (speed && speed > this.maxSpeed) {
            this.maxSpeed = speed;
            this.maxSpeedLocation = { lat, lng };
        }
        if (speed) this.speeds.push(speed);
        
        // Track elevation statistics  
        if (altitude) {
            if (!this.maxElevation || altitude > this.maxElevation) {
                this.maxElevation = altitude;
                this.maxElevationLocation = { lat, lng };
            }
            if (!this.minElevation || altitude < this.minElevation) {
                this.minElevation = altitude;
            }
        }
        
        this.lastPosition = { lat, lng };
    }
    
    // Get final trip summary
    getTripSummary() {
        const avgSpeed = this.speeds.length > 0 ? 
            this.speeds.reduce((a, b) => a + b) / this.speeds.length : 0;
        
        return {
            total_distance_km: this.totalDistance,
            max_speed_kmh: this.maxSpeed * 3.6, // m/s to km/h
            avg_speed_kmh: avgSpeed * 3.6,
            duration_seconds: Math.floor((Date.now() - this.startTime) / 1000),
            max_elevation_m: this.maxElevation ? Math.round(this.maxElevation) : null,
            min_elevation_m: this.minElevation ? Math.round(this.minElevation) : null,
            elevation_gain_m: (this.maxElevation && this.minElevation) ? 
                Math.round(this.maxElevation - this.minElevation) : null,
            max_speed_location: this.maxSpeedLocation,
            max_elevation_location: this.maxElevationLocation
        };
    }
}

// Stop tracking with mobile-calculated statistics
const tripSummary = tripTracker.getTripSummary();
fetch('/api/mobile/live-tracking/stop', {
    method: 'POST',
    headers: {
        'Authorization': 'Bearer ' + token,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        session_id: currentSessionId,
        save_trip: true,
        trip_summary: tripSummary  // ← Mobile-calculated statistics
    })
})
.then(response => response.json())
.then(data => {
    if (data.trip_saved) {
        console.log('Trip saved with mobile stats:', data.trip_id);
        showNotification(`Trip saved! Distance: ${tripSummary.total_distance_km.toFixed(1)}km`);
    }
});
```

### **2. Fallback: Basic Trip Saving**
```javascript
// Fallback for older app versions or when mobile calculation fails
fetch('/api/mobile/live-tracking/stop', {
    method: 'POST',
    headers: {
        'Authorization': 'Bearer ' + token,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        session_id: currentSessionId,
        save_trip: true  // ← Server will calculate statistics
    })
})
.then(response => response.json())
.then(data => {
    if (data.trip_saved) {
        console.log('Trip saved with server stats:', data.trip_id);
        showNotification(`Trip saved! ID: ${data.trip_id}`);
    }
});
```

### **3. Query Saved Trips**
```sql
-- Get user's recent trips
SELECT 
    t.id,
    t.train_name,
    t.duration_seconds,
    t.total_distance_km,
    t.started_at,
    t.completed_at,
    u.name as user_name
FROM trips t
JOIN users u ON t.user_id = u.id
WHERE t.user_id = 123
ORDER BY t.completed_at DESC
LIMIT 10;
```

### **4. Trip Analytics**
```sql
-- Trip statistics
SELECT 
    COUNT(*) as total_trips,
    AVG(duration_seconds) as avg_duration,
    AVG(total_distance_km) as avg_distance,
    MAX(max_speed_kmh) as top_speed,
    SUM(total_distance_km) as total_km_traveled
FROM trips
WHERE user_id = 123
  AND completed_at >= DATE_SUB(NOW(), INTERVAL 30 DAY);
```

---

## ⚠️ Important Notes

### **Statistics Calculation Approaches**
- **Mobile-Calculated** ⭐: Real-time calculation on device with high GPS frequency
  - More accurate distance and speed measurements
  - Better elevation tracking with device sensors
  - Reduces server CPU load
  - Works with offline tracking

- **Server-Calculated** 🔄: Fallback calculation from S3 GPS history
  - Uses Haversine formula for distance calculation
  - Less accurate due to lower GPS sampling rate
  - CPU intensive on server
  - Good compatibility with older clients

### **Data Size Considerations**
- **Tracking Data**: Can be large (100KB+ for long trips)
- **Route Coordinates**: Simplified version for efficient map rendering
- **Storage**: LONGTEXT field supports up to 4GB per record

### **Performance Tips**
- **Preferred**: Mobile apps calculate statistics in real-time during tracking
- Trip saving happens **after** user is removed from real-time tracking
- S3 data is read **before** file cleanup to ensure data availability  
- Database transaction ensures data consistency

### **Error Handling**
```go
// Trip saving is optional - session stops even if trip save fails
if req.SaveTrip != nil && *req.SaveTrip {
    tripID = h.saveUserTrip(session, user.ID)
    if tripID == nil {
        fmt.Printf("WARNING: Trip saving failed, but session stopped successfully")
    }
}
```

---

## 🗄️ Database Query Examples

### **Find Trips by Distance**
```sql
-- Top 10 longest trips
SELECT id, train_name, total_distance_km, duration_seconds, user_id
FROM trips 
WHERE total_distance_km > 0
ORDER BY total_distance_km DESC 
LIMIT 10;
```

### **Trip Duration Analysis**
```sql
-- Average trip duration by train
SELECT 
    train_name,
    COUNT(*) as trip_count,
    AVG(duration_seconds) as avg_duration_sec,
    AVG(total_distance_km) as avg_distance_km
FROM trips 
WHERE duration_seconds > 0
GROUP BY train_name
ORDER BY avg_distance_km DESC;
```

### **User Travel Patterns**
```sql
-- Most active users by distance traveled
SELECT 
    u.name,
    COUNT(t.id) as total_trips,
    SUM(t.total_distance_km) as total_km,
    AVG(t.total_distance_km) as avg_trip_km
FROM trips t
JOIN users u ON t.user_id = u.id
WHERE t.completed_at >= DATE_SUB(NOW(), INTERVAL 90 DAY)
GROUP BY u.id, u.name
ORDER BY total_km DESC
LIMIT 20;
```

---

## 📱 Mobile App Integration

### **UI Flow**
1. **During Tracking**: Show "Save Trip" toggle in stop dialog
2. **On Stop**: Display saving progress
3. **After Save**: Show trip summary with saved trip ID
4. **Trip History**: List saved trips with details

### **Offline Handling**
- Trip data is preserved in S3 until session cleanup
- Retry trip saving if network fails during stop request
- Local storage can cache trip data for manual retry

---

*This documentation covers the complete trip saving functionality. For questions or improvements, check the source code in `handlers/simple_live_tracking.go`.*