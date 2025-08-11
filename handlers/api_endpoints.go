package handlers

import (
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

// GetSchedules - GET /api/schedules
// Returns all schedule details with train and station info, matching Laravel API structure
func (h *APIEndpointsHandler) GetSchedules(c *gin.Context) {
	var scheduleDetails []models.ScheduleDetail
	
	result := h.db.Preload("Train").
		Preload("Station").
		Select("schedule_detail_id, train_id, station_id, stop_sequence, arrival_time, departure_time, is_pass_through, remarks").
		Order("train_id, stop_sequence").
		Find(&scheduleDetails)
		
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch schedules",
			"detail": result.Error.Error(),
		})
		return
	}

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

	// Then, separately load railway lines for each operational route
	for i := range operationalRoutes {
		var railwayLines []models.RailwayLine
		err := h.db.Table("operational_route_railway_line").
			Select("railway_lines.id, railway_lines.name, railway_lines.electrification, railway_lines.train_types_allowed, railway_lines.from_station_id, railway_lines.to_station_id, railway_lines.created_at, railway_lines.updated_at").
			Joins("JOIN railway_lines ON railway_lines.id = operational_route_railway_line.railway_line_id").
			Preload("FromStation").
			Preload("ToStation").
			Where("operational_route_railway_line.operational_route_id = ?", operationalRoutes[i].ID).
			Find(&railwayLines).Error
		
		if err != nil {
			// Log the error but continue without railway lines
			continue
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

	// Separately load railway lines without geometry to avoid parsing issues
	var railwayLines []models.RailwayLine
	err = h.db.Table("operational_route_railway_line").
		Select("railway_lines.id, railway_lines.name, railway_lines.electrification, railway_lines.train_types_allowed, railway_lines.from_station_id, railway_lines.to_station_id, railway_lines.created_at, railway_lines.updated_at").
		Joins("JOIN railway_lines ON railway_lines.id = operational_route_railway_line.railway_line_id").
		Preload("FromStation").
		Preload("ToStation").
		Where("operational_route_railway_line.operational_route_id = ?", operationalRoute.ID).
		Find(&railwayLines).Error
	
	if err == nil {
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