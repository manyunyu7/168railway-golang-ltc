# CartoDB Tile Proxy Guide

This proxy bypasses CartoDB blocking issues by serving tiles through your own domain with intelligent caching.

## 🚀 Features

- **High Performance**: Memory + File caching for 100k+ requests/day
- **Smart Caching**: 24-hour TTL with automatic cleanup
- **Retina Support**: @2x tiles for high-density displays
- **CORS Ready**: Proper headers for web map libraries
- **Monitoring**: Cache stats and health endpoints

## 📡 Endpoints

### Tile Proxy Endpoints
Replace CartoDB URLs with your proxy URLs:

**Original CartoDB:**
```
https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}.png
https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png
```

**Your Proxy URLs:**
```
https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}.png
https://go-ltc.trainradar35.com/api/tiles/dark_all/{z}/{x}/{y}.png
```

**Retina Support:**
```
https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}@2x.png
https://go-ltc.trainradar35.com/api/tiles/dark_all/{z}/{x}/{y}@2x.png
```

### Monitoring Endpoints
```
GET /api/tiles/health   - Proxy health check
GET /api/tiles/stats    - Cache statistics
```

## 🗺️ Frontend Integration

### Leaflet.js Example
```javascript
// Replace CartoDB tiles with your proxy
const lightTiles = L.tileLayer(
  'https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}.png',
  {
    attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>, &copy; <a href="https://carto.com/attributions">CARTO</a>',
    maxZoom: 19
  }
);

const darkTiles = L.tileLayer(
  'https://go-ltc.trainradar35.com/api/tiles/dark_all/{z}/{x}/{y}.png',
  {
    attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>, &copy; <a href="https://carto.com/attributions">CARTO</a>',
    maxZoom: 19
  }
);

// Retina-aware tiles
const lightTilesRetina = L.tileLayer(
  'https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}' + 
  (L.Browser.retina ? '@2x.png' : '.png'),
  {
    attribution: '&copy; <a href="http://www.openstreetmap.org/copyright">OpenStreetMap</a>, &copy; <a href="https://carto.com/attributions">CARTO</a>',
    maxZoom: 19
  }
);
```

### OpenLayers Example
```javascript
import {OSM, XYZ} from 'ol/source';

const lightSource = new XYZ({
  url: 'https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}.png'
});

const darkSource = new XYZ({
  url: 'https://go-ltc.trainradar35.com/api/tiles/dark_all/{z}/{x}/{y}.png'
});
```

### Mapbox GL Example
```javascript
const lightStyle = {
  "version": 8,
  "sources": {
    "carto-light": {
      "type": "raster",
      "tiles": ["https://go-ltc.trainradar35.com/api/tiles/light_all/{z}/{x}/{y}.png"],
      "tileSize": 256
    }
  },
  "layers": [{
    "id": "carto-light-layer",
    "type": "raster",
    "source": "carto-light"
  }]
};
```

## 🚀 Performance Optimizations

### Caching Strategy
- **Memory Cache**: 1000 hot tiles in RAM
- **File Cache**: Unlimited tiles on disk (24h TTL)
- **Automatic Cleanup**: Hourly cleanup of expired tiles
- **Smart Eviction**: LRU eviction for memory cache

### Load Balancing
- **Subdomain Rotation**: Distributes load across CartoDB servers
- **Connection Pooling**: Efficient HTTP client with connection reuse
- **Timeout Handling**: 30-second timeout with retry logic

### Cache Directory Structure
```
./cache/tiles/
├── ab/
│   └── cd/
│       └── abcd123...png
└── ef/
    └── gh/
        └── efgh456...png
```

## 📊 Cache Statistics API Response
```json
{
  "memory_cache_entries": 847,
  "file_cache_entries": 15420,
  "cache_ttl_hours": 24,
  "cache_directory": "./cache/tiles"
}
```

## 🛠️ Production Deployment

### Build & Deploy
```bash
# Build application
go build -o go-ltc cmd/main.go

# Restart service
sudo systemctl restart go-ltc

# Check status
sudo systemctl status go-ltc
```

### Monitoring
```bash
# Check proxy health
curl https://go-ltc.trainradar35.com/api/tiles/health

# Check cache stats  
curl https://go-ltc.trainradar35.com/api/tiles/stats

# Test tile fetching
curl https://go-ltc.trainradar35.com/api/tiles/light_all/10/512/512.png
```

### Disk Space Management
- **Expected Usage**: ~2GB cache for 100k requests/day
- **Cache Location**: `./cache/tiles/` directory
- **Automatic Cleanup**: Files older than 24 hours are removed
- **Manual Cleanup**: `rm -rf ./cache/tiles/*` if needed

## 🔧 Configuration Options

The proxy is pre-configured for optimal performance:

- **Cache TTL**: 24 hours (good balance of freshness vs performance)
- **Memory Cache**: 1000 tiles (~ 50MB RAM usage)
- **HTTP Timeout**: 30 seconds
- **Concurrent Connections**: 100 max, 10 per host

## 📈 Expected Performance

- **Cache Hit Rate**: 85-90% after warm-up
- **Response Time**: <50ms for cached tiles, <2s for new tiles
- **Throughput**: 1000+ requests/second with proper caching
- **Storage Growth**: ~2GB/month for active usage

## 🔍 Troubleshooting

### Common Issues

1. **Tile Loading Errors**: Check network connectivity to CartoDB
2. **Cache Not Working**: Verify `./cache/tiles/` directory permissions
3. **High Memory Usage**: Memory cache automatically limits to 1000 tiles
4. **Slow Performance**: Check cache hit rate via `/api/tiles/stats`

### Debug Commands
```bash
# Check tile cache directory
ls -la ./cache/tiles/

# Monitor real-time requests
tail -f /var/log/go-ltc/access.log | grep "/api/tiles/"

# Test specific coordinates
curl -v "https://go-ltc.trainradar35.com/api/tiles/light_all/10/512/512.png"
```

## 📝 Notes

- Tiles are cached for 24 hours to balance freshness with performance
- Retina (@2x) tiles are supported for high-DPI displays
- CORS headers are properly configured for web usage
- The proxy respects CartoDB's terms of service by not overwhelming their servers
- Cache cleanup runs automatically every hour to manage disk space