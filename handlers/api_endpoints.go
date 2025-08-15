package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
)

type APIEndpointsHandler struct {
	db *gorm.DB
}

func NewAPIEndpointsHandler(db *gorm.DB) *APIEndpointsHandler {
	return &APIEndpointsHandler{
		db: db,
	}
}

// GetStations - GET /api/stations
// Returns all stations with platforms, matching Laravel API structure
func (h *APIEndpointsHandler) GetStations(c *gin.Context) {
	var stations []models.Station
	
	result := h.db.Preload("Platforms").
		Select("station_id, station_code, station_name, latitude, longitude").
		Find(&stations)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch stations",
			"detail": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stations)
}

// ScheduleResponse - Lightweight DTO for frontend consumption
type ScheduleResponse struct {
	ScheduleDetailID uint    `json:"schedule_detail_id"`
	TrainID          uint    `json:"train_id"`
	StationID        uint    `json:"station_id"`
	StopSequence     int     `json:"stop_sequence"`
	ArrivalTime      *string `json:"arrival_time"`
	DepartureTime    *string `json:"departure_time"`
	IsPassThrough    bool    `json:"is_pass_through"`
	Remarks          *string `json:"remarks"`
}

// GetSchedules - GET /api/schedules
// Returns lightweight schedule data without embedded objects, optimized for frontend
// Supports filtering by station_id: /api/schedules?station_id=1
// Supports pagination: /api/schedules?page=1&limit=100
func (h *APIEndpointsHandler) GetSchedules(c *gin.Context) {
	var scheduleDetails []ScheduleResponse
	
	// Parse pagination parameters - only paginate if explicitly requested
	var usePagination bool
	var page, limit, offset int
	
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
			usePagination = true
		}
	}
	
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50000 {
			limit = l
			usePagination = true
		}
	}
	
	// Set defaults only if pagination is requested
	if usePagination {
		if page == 0 {
			page = 1
		}
		if limit == 0 {
			limit = 1000
		}
		offset = (page - 1) * limit
	}
	
	// Build optimized query - select only needed fields
	query := h.db.Table("schedule_details").
		Select("schedule_detail_id, train_id, station_id, stop_sequence, arrival_time, departure_time, is_pass_through, remarks")
	
	// Check for station_id filter parameter
	stationID := c.Query("station_id")
	if stationID != "" {
		// Filter by specific station - only return schedules for trains that pass through this station
		query = query.Where("train_id IN (?)", 
			h.db.Table("schedule_details").
				Select("DISTINCT train_id").
				Where("station_id = ?", stationID))
	}
	
	// Apply pagination and ordering only if requested
	query = query.Order("train_id, stop_sequence")
	if usePagination {
		query = query.Offset(offset).Limit(limit)
	}
	
	result := query.Find(&scheduleDetails)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch schedules",
			"detail": result.Error.Error(),
		})
		return
	}

	// Get total count for pagination metadata
	var totalCount int64
	countQuery := h.db.Table("schedule_details")
	if stationID != "" {
		countQuery = countQuery.Where("train_id IN (?)", 
			h.db.Table("schedule_details").
				Select("DISTINCT train_id").
				Where("station_id = ?", stationID))
	}
	countQuery.Count(&totalCount)
	
	// Add metadata headers
	c.Header("X-Total-Count", fmt.Sprintf("%d", totalCount))
	
	// Add pagination headers only if pagination is used
	if usePagination {
		totalPages := (totalCount + int64(limit) - 1) / int64(limit)
		c.Header("X-Total-Pages", fmt.Sprintf("%d", totalPages))
		c.Header("X-Current-Page", fmt.Sprintf("%d", page))
		c.Header("X-Per-Page", fmt.Sprintf("%d", limit))
	}
	
	if stationID != "" {
		// Get unique train count for this station
		var trainCount int64
		h.db.Table("schedule_details").
			Where("station_id = ?", stationID).
			Distinct("train_id").
			Count(&trainCount)
			
		// Add filter metadata headers
		c.Header("X-Station-Filter", stationID)
		c.Header("X-Trains-Count", fmt.Sprintf("%d", trainCount))
	}
	c.Header("X-Records-Count", fmt.Sprintf("%d", len(scheduleDetails)))

	c.JSON(http.StatusOK, scheduleDetails)
}

// GetOperationalRoutesPathway - GET /api/operational-routes-pathway  
// Returns all operational routes with railway lines and station info, matching Laravel API structure
func (h *APIEndpointsHandler) GetOperationalRoutesPathway(c *gin.Context) {
	var operationalRoutes []models.OperationalRoute
	
	// First, get operational routes without railway lines to avoid geometry parsing issues
	result := h.db.Preload("StartStation").
		Preload("EndStation").
		Find(&operationalRoutes)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch operational routes",
			"detail": result.Error.Error(),
		})
		return
	}

	// Then, separately load railway lines for each operational route with pivot data
	for i := range operationalRoutes {
		var railwayLines []models.RailwayLine
		var pivotData []struct {
			models.RailwayLine
			OperationalRouteID uint `gorm:"column:operational_route_id"`
			Sequence          *int `gorm:"column:sequence"`
			IsReversed        *bool `gorm:"column:is_reversed"`
		}
		
		err := h.db.Table("operational_route_railway_line").
			Select("railway_lines.*, operational_route_railway_line.operational_route_id, operational_route_railway_line.sequence, operational_route_railway_line.is_reversed").
			Joins("JOIN railway_lines ON railway_lines.id = operational_route_railway_line.railway_line_id").
			Where("operational_route_railway_line.operational_route_id = ?", operationalRoutes[i].ID).
			Find(&pivotData).Error
		
		if err != nil {
			// Log the error but continue without railway lines
			continue
		}
		
		// Convert pivot data to railway lines with pivot info
		railwayLines = make([]models.RailwayLine, len(pivotData))
		for j, data := range pivotData {
			railwayLines[j] = data.RailwayLine
			railwayLines[j].Pivot = &models.OperationalRoutePivot{
				OperationalRouteID: data.OperationalRouteID,
				RailwayLineID: data.ID,
				Sequence: data.Sequence,
				IsReversed: data.IsReversed,
			}
		}
		
		// Load station relationships separately
		for j := range railwayLines {
			if railwayLines[j].FromStationID != nil {
				h.db.First(&railwayLines[j].FromStation, *railwayLines[j].FromStationID)
			}
			if railwayLines[j].ToStationID != nil {
				h.db.First(&railwayLines[j].ToStation, *railwayLines[j].ToStationID)
			}
		}
		
		operationalRoutes[i].RailwayLines = railwayLines
	}

	c.JSON(http.StatusOK, operationalRoutes)
}

// GetStationByID - GET /api/stations/:id
// Returns a single station by ID
func (h *APIEndpointsHandler) GetStationByID(c *gin.Context) {
	stationID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid station ID",
		})
		return
	}

	var station models.Station
	result := h.db.Preload("Platforms").
		First(&station, stationID)
		
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Station not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch station",
				"detail": result.Error.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, station)
}

// GetTrainSchedule - GET /api/trains/:id/schedule
// Returns schedule details for a specific train
func (h *APIEndpointsHandler) GetTrainSchedule(c *gin.Context) {
	trainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid train ID",
		})
		return
	}

	var scheduleDetails []models.ScheduleDetail
	result := h.db.Preload("Station").
		Where("train_id = ?", trainID).
		Order("stop_sequence").
		Find(&scheduleDetails)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch train schedule",
			"detail": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, scheduleDetails)
}

// GetOperationalRouteByID - GET /api/operational-routes/:id
// Returns a single operational route by ID
func (h *APIEndpointsHandler) GetOperationalRouteByID(c *gin.Context) {
	routeID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid route ID",
		})
		return
	}

	var operationalRoute models.OperationalRoute
	result := h.db.Preload("StartStation").
		Preload("EndStation").
		First(&operationalRoute, routeID)
		
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Operational route not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch operational route",
				"detail": result.Error.Error(),
			})
		}
		return
	}

	// Separately load railway lines with pivot data for full compatibility
	var pivotData []struct {
		models.RailwayLine
		OperationalRouteID uint `gorm:"column:operational_route_id"`
		Sequence          *int `gorm:"column:sequence"`
		IsReversed        *bool `gorm:"column:is_reversed"`
	}
	
	err = h.db.Table("operational_route_railway_line").
		Select("railway_lines.*, operational_route_railway_line.operational_route_id, operational_route_railway_line.sequence, operational_route_railway_line.is_reversed").
		Joins("JOIN railway_lines ON railway_lines.id = operational_route_railway_line.railway_line_id").
		Where("operational_route_railway_line.operational_route_id = ?", operationalRoute.ID).
		Find(&pivotData).Error
	
	if err == nil {
		// Convert pivot data to railway lines with pivot info
		railwayLines := make([]models.RailwayLine, len(pivotData))
		for j, data := range pivotData {
			railwayLines[j] = data.RailwayLine
			railwayLines[j].Pivot = &models.OperationalRoutePivot{
				OperationalRouteID: data.OperationalRouteID,
				RailwayLineID: data.ID,
				Sequence: data.Sequence,
				IsReversed: data.IsReversed,
			}
		}
		
		// Load station relationships separately
		for j := range railwayLines {
			if railwayLines[j].FromStationID != nil {
				h.db.First(&railwayLines[j].FromStation, *railwayLines[j].FromStationID)
			}
			if railwayLines[j].ToStationID != nil {
				h.db.First(&railwayLines[j].ToStation, *railwayLines[j].ToStationID)
			}
		}
		
		operationalRoute.RailwayLines = railwayLines
	}

	c.JSON(http.StatusOK, operationalRoute)
}

// SearchStations - GET /api/stations/search?query=
// Search stations by name or code
func (h *APIEndpointsHandler) SearchStations(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Query parameter is required",
		})
		return
	}

	var stations []models.Station
	result := h.db.Where("station_name LIKE ? OR station_code LIKE ?", 
		"%"+query+"%", "%"+query+"%").
		Select("station_id, station_code, station_name, latitude, longitude").
		Limit(50).
		Find(&stations)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to search stations",
			"detail": result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stations)
}