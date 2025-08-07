# 🚀 API Upgrade Notice: WebSocket Real-time Updates Available!

## 📢 **Important Update for Developers**

We've upgraded the live train tracking API with **real-time WebSocket support**! This provides better performance and user experience.

---

## ⚡ **What's New: WebSocket Real-time Updates**

### **New WebSocket Endpoint**
```
wss://go-ltc.trainradar35.com/ws/trains
```

### **Benefits of Upgrading to WebSocket:**
- ✅ **Real-time updates** (every 5 seconds automatically)
- ✅ **Individual passenger positions** with exact GPS coordinates  
- ✅ **Lower bandwidth usage** (no more polling)
- ✅ **Instant notifications** when trains move or passengers join/leave
- ✅ **Better user experience** with smooth real-time tracking
- ✅ **Reduced server load** and battery usage

---

## 🔄 **Migration Guide**

### **Option 1: WebSocket Only (Recommended)**
```javascript
// Connect to WebSocket for real-time updates
const ws = new WebSocket('wss://go-ltc.trainradar35.com/ws/trains');

ws.onopen = () => {
    console.log('Connected to real-time train updates');
};

ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    
    if (message.type === 'initial_data') {
        // Full trains list on connection
        displayTrains(message.data.trains);
    } 
    else if (message.type === 'train_updates') {
        // Real-time updates with individual passenger positions
        updateTrainsRealtime(message.data);
    }
};
```

### **Option 2: WebSocket with HTTP Fallback (Most Robust)**
```javascript
let ws = null;
let fallbackInterval = null;

function connectToRealtime() {
    ws = new WebSocket('wss://go-ltc.trainradar35.com/ws/trains');
    
    ws.onopen = () => {
        console.log('WebSocket connected - real-time updates active');
        clearInterval(fallbackInterval); // Stop HTTP polling
    };
    
    ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        if (message.type === 'train_updates') {
            updateTrains(message.data); // Real-time data
        }
    };
    
    ws.onerror = () => {
        console.log('WebSocket failed, using HTTP fallback');
        startHttpPolling(); // Fallback to old method
    };
}

function startHttpPolling() {
    fallbackInterval = setInterval(() => {
        fetch('https://go-ltc.trainradar35.com/api/active-train-list')
            .then(r => r.json())
            .then(data => updateTrains(data.trains));
    }, 5000);
}
```

---

## 📊 **Data Structure Comparison**

### **HTTP Response (Current)**
```json
{
  "trains": [
    {
      "trainId": "KA123",
      "passengerCount": 5,
      "lastUpdate": "2025-08-07T18:00:00Z",
      "status": "active"
    }
  ],
  "total": 1
}
```

### **WebSocket Response (NEW - More Detailed)**
```json
{
  "type": "train_updates",
  "data": [
    {
      "trainNumber": "KA123",
      "passengerCount": 5,
      "averagePosition": {"lat": -6.2088, "lng": 106.8456},
      "passengers": [
        {
          "userID": 123,
          "lat": -6.2088,
          "lng": 106.8456,
          "timestamp": 1691427600000,
          "status": "active",
          "clientType": "mobile"
        }
      ],
      "route": "Jakarta - Bandung",
      "dataSource": "live-gps",
      "lastUpdate": "2025-08-07T18:00:00Z",
      "status": "active"
    }
  ]
}
```

---

## 🛠️ **HTTP Endpoints Still Available**

**Don't worry!** All existing HTTP endpoints continue to work:

- ✅ `GET /api/active-train-list` - Still available
- ✅ `GET /api/train/{trainNumber}` - Still available  
- ✅ `GET /health` - Still available

**You can migrate at your own pace!**

---

## 🎯 **WebSocket Message Types**

| Message Type | Description | When Sent |
|--------------|-------------|-----------|
| `initial_data` | Full trains list | On connection |
| `train_updates` | Real-time position updates | Every 5 seconds |
| `pong` | Response to ping | On ping request |

### **Sending Messages to Server**
```javascript
// Send ping to keep connection alive
ws.send(JSON.stringify({
    type: 'ping',
    data: {}
}));

// Subscribe to specific train (optional)
ws.send(JSON.stringify({
    type: 'subscribe_train',
    data: 'KA123'
}));
```

---

## ⚙️ **Connection Management**

### **Reconnection Logic**
```javascript
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;

function connectWithRetry() {
    const ws = new WebSocket('wss://go-ltc.trainradar35.com/ws/trains');
    
    ws.onclose = () => {
        if (reconnectAttempts < maxReconnectAttempts) {
            reconnectAttempts++;
            setTimeout(() => {
                console.log(`Reconnecting... attempt ${reconnectAttempts}`);
                connectWithRetry();
            }, 2000 * reconnectAttempts); // Exponential backoff
        } else {
            console.log('Max reconnection attempts reached, falling back to HTTP');
            startHttpPolling();
        }
    };
    
    ws.onopen = () => {
        reconnectAttempts = 0; // Reset on successful connection
    };
}
```

---

## 🔍 **Testing & Debugging**

### **Check WebSocket Availability**
```javascript
// Test endpoint to check WebSocket status
fetch('https://go-ltc.trainradar35.com/api/websocket-info')
    .then(r => r.json())
    .then(info => {
        console.log('WebSocket available:', info.websocket_available);
        console.log('WebSocket URL:', info.websocket_url);
        console.log('Benefits:', info.upgrade_benefits);
    });
```

### **Browser Developer Tools**
- Open Network tab → WS (WebSocket) filter
- Look for connection to `/ws/trains`
- Monitor real-time message flow

---

## 📞 **Need Help?**

### **Common Issues:**

**Q: WebSocket connection fails**
A: The system automatically falls back to HTTP polling. No action needed.

**Q: Getting different data structure**
A: WebSocket provides more detailed data (individual passenger positions). Use `message.data` array.

**Q: Want to keep using HTTP only**
A: No problem! All HTTP endpoints remain fully functional.

### **Support:**
- Check server logs for WebSocket connection status
- Test with the demo page: `websocket_example.html`
- HTTP fallback ensures your app keeps working

---

## 🎉 **Ready to Upgrade?**

**Start small:** Add WebSocket as a fallback to your existing HTTP implementation.

**Go big:** Replace all polling with real-time WebSocket updates for the best user experience!

The choice is yours - both approaches are fully supported! 🚂⚡

---
*Last updated: August 2025 | API Version: v2.0-websocket*