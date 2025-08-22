package handlers

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type TileProxyHandler struct {
	cacheDir    string
	cacheTTL    time.Duration
	memoryCache map[string]*CacheEntry
	cacheMutex  sync.RWMutex
	client      *http.Client
}

type CacheEntry struct {
	Data      []byte
	CachedAt  time.Time
	ExpiresAt time.Time
}

// NewTileProxyHandler creates a new tile proxy handler with caching
func NewTileProxyHandler() *TileProxyHandler {
	cacheDir := "./cache/tiles"
	os.MkdirAll(cacheDir, 0755)

	handler := &TileProxyHandler{
		cacheDir:    cacheDir,
		cacheTTL:    24 * time.Hour, // Cache tiles for 24 hours
		memoryCache: make(map[string]*CacheEntry),
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}

	// Start cache cleanup routine
	go handler.startCacheCleanup()

	return handler
}

// ProxyCartoDB handles CartoDB tile requests with caching
func (h *TileProxyHandler) ProxyCartoDB(c *gin.Context) {
	style := c.Param("style")   // light_all or dark_all
	z := c.Param("z")           // zoom level
	x := c.Param("x")           // x coordinate
	y := c.Param("y")           // y coordinate (may include @2x.png)
	
	// Validate style parameter
	if style != "light_all" && style != "dark_all" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid style. Use 'light_all' or 'dark_all'"})
		return
	}

	// Validate coordinates
	if !isValidCoordinate(z) || !isValidCoordinate(x) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid coordinates"})
		return
	}

	// Handle retina tiles (@2x)
	isRetina := strings.HasSuffix(y, "@2x.png")
	if isRetina {
		y = strings.TrimSuffix(y, "@2x.png")
	} else {
		y = strings.TrimSuffix(y, ".png")
	}

	if !isValidCoordinate(y) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid y coordinate"})
		return
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%s_%s_%s_%s", style, z, x, y)
	if isRetina {
		cacheKey += "_retina"
	}

	// Try memory cache first
	h.cacheMutex.RLock()
	if entry, exists := h.memoryCache[cacheKey]; exists && time.Now().Before(entry.ExpiresAt) {
		h.cacheMutex.RUnlock()
		h.serveCachedTile(c, entry.Data)
		return
	}
	h.cacheMutex.RUnlock()

	// Try file cache
	cacheFilePath := h.getCacheFilePath(cacheKey)
	if fileInfo, err := os.Stat(cacheFilePath); err == nil {
		// Check if cache file is still valid
		if time.Since(fileInfo.ModTime()) < h.cacheTTL {
			if data, err := os.ReadFile(cacheFilePath); err == nil {
				// Update memory cache
				h.cacheMutex.Lock()
				h.memoryCache[cacheKey] = &CacheEntry{
					Data:      data,
					CachedAt:  fileInfo.ModTime(),
					ExpiresAt: fileInfo.ModTime().Add(h.cacheTTL),
				}
				h.cacheMutex.Unlock()

				h.serveCachedTile(c, data)
				return
			}
		}
	}

	// Fetch from CartoDB
	tileData, err := h.fetchTileFromCartoDB(style, z, x, y, isRetina)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch tile: %v", err)})
		return
	}

	// Cache the tile
	go h.cacheTile(cacheKey, cacheFilePath, tileData)

	// Serve the tile
	h.serveCachedTile(c, tileData)
}

// fetchTileFromCartoDB fetches tile from CartoDB servers
func (h *TileProxyHandler) fetchTileFromCartoDB(style, z, x, y string, isRetina bool) ([]byte, error) {
	// Use subdomain rotation for better performance
	subdomains := []string{"a", "b", "c", "d"}
	subdomain := subdomains[0] // Simple rotation, could be improved

	// Build URL
	var tileURL string
	if isRetina {
		tileURL = fmt.Sprintf("https://%s.basemaps.cartocdn.com/%s/%s/%s/%s@2x.png", 
			subdomain, style, z, x, y)
	} else {
		tileURL = fmt.Sprintf("https://%s.basemaps.cartocdn.com/%s/%s/%s/%s.png", 
			subdomain, style, z, x, y)
	}

	// Create request
	req, err := http.NewRequest("GET", tileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set proper headers to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://go-ltc.trainradar35.com/")
	req.Header.Set("Accept", "image/png,image/*,*/*;q=0.8")

	// Make request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tile: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CartoDB returned status: %d", resp.StatusCode)
	}

	// Read response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	return data, nil
}

// serveCachedTile serves tile data with proper headers
func (h *TileProxyHandler) serveCachedTile(c *gin.Context, data []byte) {
	c.Header("Content-Type", "image/png")
	c.Header("Cache-Control", "public, max-age=86400") // 24 hours
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
	c.Data(http.StatusOK, "image/png", data)
}

// cacheTile caches tile in both memory and file system
func (h *TileProxyHandler) cacheTile(cacheKey, filePath string, data []byte) {
	now := time.Now()
	expiresAt := now.Add(h.cacheTTL)

	// Update memory cache
	h.cacheMutex.Lock()
	h.memoryCache[cacheKey] = &CacheEntry{
		Data:      data,
		CachedAt:  now,
		ExpiresAt: expiresAt,
	}
	
	// Limit memory cache size (keep last 1000 tiles)
	if len(h.memoryCache) > 1000 {
		// Remove oldest entries
		var oldestKey string
		var oldestTime time.Time = time.Now()
		
		for key, entry := range h.memoryCache {
			if entry.CachedAt.Before(oldestTime) {
				oldestTime = entry.CachedAt
				oldestKey = key
			}
		}
		
		if oldestKey != "" {
			delete(h.memoryCache, oldestKey)
		}
	}
	h.cacheMutex.Unlock()

	// Save to file cache
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err == nil {
		os.WriteFile(filePath, data, 0644)
	}
}

// getCacheFilePath generates cache file path for a given key
func (h *TileProxyHandler) getCacheFilePath(cacheKey string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(cacheKey)))
	return filepath.Join(h.cacheDir, hash[:2], hash[2:4], hash+".png")
}

// startCacheCleanup starts a goroutine to clean up expired cache entries
func (h *TileProxyHandler) startCacheCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		h.cleanupCache()
	}
}

// cleanupCache removes expired entries from memory and file cache
func (h *TileProxyHandler) cleanupCache() {
	now := time.Now()

	// Clean memory cache
	h.cacheMutex.Lock()
	for key, entry := range h.memoryCache {
		if now.After(entry.ExpiresAt) {
			delete(h.memoryCache, key)
		}
	}
	h.cacheMutex.Unlock()

	// Clean file cache
	filepath.Walk(h.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if time.Since(info.ModTime()) > h.cacheTTL {
			os.Remove(path)
		}

		return nil
	})
}

// GetCacheStats returns cache statistics
func (h *TileProxyHandler) GetCacheStats(c *gin.Context) {
	h.cacheMutex.RLock()
	memoryCount := len(h.memoryCache)
	h.cacheMutex.RUnlock()

	// Count file cache entries
	fileCount := 0
	filepath.Walk(h.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		fileCount++
		return nil
	})

	c.JSON(http.StatusOK, gin.H{
		"memory_cache_entries": memoryCount,
		"file_cache_entries":   fileCount,
		"cache_ttl_hours":      int(h.cacheTTL.Hours()),
		"cache_directory":      h.cacheDir,
	})
}

// isValidCoordinate validates if a coordinate string is numeric
func isValidCoordinate(coord string) bool {
	_, err := strconv.Atoi(coord)
	return err == nil
}

// Health check for tile proxy
func (h *TileProxyHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "tile-proxy",
		"uptime":  time.Since(time.Now()).String(),
	})
}