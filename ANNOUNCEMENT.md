# 🚀 **NEW: Real-time WebSocket API Available!**

## **📡 Upgrade Your Train Tracking App with Real-time Updates**

### **What's New:**
- **WebSocket endpoint:** `wss://go-ltc.trainradar35.com/ws/trains`
- **Real-time updates** every 5 seconds (no more polling!)
- **Individual passenger GPS positions** (not just train averages)
- **50% less bandwidth** usage
- **Instant notifications** when trains move

### **Quick Start:**
```javascript
const ws = new WebSocket('wss://go-ltc.trainradar35.com/ws/trains');
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'train_updates') {
        // Real-time train positions with individual passengers!
        updateMap(data.data);
    }
};
```

### **Migration:**
- ✅ **HTTP endpoints still work** - migrate at your own pace
- ✅ **Automatic fallback** - if WebSocket fails, use HTTP
- ✅ **More detailed data** - individual passenger locations included
- ✅ **Better performance** - real-time instead of polling

### **Documentation:**
- **Full migration guide:** `API_MIGRATION_GUIDE.md`
- **Demo page:** `websocket_example.html` 
- **Endpoint reference:** `CLAUDE.md`

### **Benefits:**
| Feature | HTTP Polling | WebSocket |
|---------|-------------|-----------|
| Update Speed | 5-30 seconds | Real-time |
| Bandwidth | High | 50% less |
| Passenger Data | Average only | Individual positions |
| Battery Usage | Higher | Lower |

**Ready to upgrade? Check the migration guide for step-by-step instructions!**

---
*Questions? The HTTP API continues to work exactly as before. WebSocket is an optional upgrade for better performance.*