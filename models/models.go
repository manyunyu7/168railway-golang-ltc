package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// User model matching Laravel users table
type User struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	Name                string    `json:"name"`
	Email               string    `json:"email" gorm:"unique"`
	EmailVerifiedAt     *time.Time `json:"email_verified_at"`
	Password            string    `json:"-"`
	RememberToken       *string   `json:"-"`
	NIP                 *string   `json:"nip"`
	Username            *string   `json:"username" gorm:"unique"`
	StationID           *uint     `json:"station_id"`
	StationName         *string   `json:"station_name"`
	Role                string    `json:"role" gorm:"default:user"`
	PhoneNumber         *string   `json:"phone_number"`
	ProfilePhoto        *string   `json:"profile_photo"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// PersonalAccessToken model matching Laravel personal_access_tokens table
type PersonalAccessToken struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	TokenableID uint     `json:"tokenable_id"`
	TokenableType string `json:"tokenable_type"`
	Name       string    `json:"name"`
	Token      string    `json:"token" gorm:"index"`
	Abilities  *string   `json:"abilities"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// LiveSession represents cached session data
type LiveSession struct {
	UserID       uint      `json:"user_id"`
	UserType     string    `json:"user_type"`
	ClientType   string    `json:"client_type"`
	TrainID      uint      `json:"train_id"`
	TrainNumber  string    `json:"train_number"`
	StartedAt    time.Time `json:"started_at"`
	FilePath     string    `json:"file_path"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// TrainData represents the JSON structure stored in S3
type TrainData struct {
	TrainID         string         `json:"trainId"`
	Route           string         `json:"route"`
	PassengerCount  int           `json:"passengerCount"`
	AveragePosition Position      `json:"averagePosition"`
	Passengers      []Passenger   `json:"passengers"`
	LastUpdate      string        `json:"lastUpdate"`
	Status          string        `json:"status"`
	DataSource      string        `json:"dataSource"`
}

type Position struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// UserStatus represents a passenger's current status message
type UserStatus struct {
	Emoji     string `json:"emoji"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type Passenger struct {
	UserID      uint    `json:"userId"`
	Name        string  `json:"name"`        // User's full name
	Username    string  `json:"username"`    // User's username (if available)
	StationName string  `json:"stationName"` // Station name from stations table lookup
	UserType    string  `json:"userType"`
	ClientType  string  `json:"clientType"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Timestamp   int64   `json:"timestamp"`
	SessionID   string  `json:"sessionId"`
	Accuracy    *float64    `json:"accuracy,omitempty"`
	Speed       *float64    `json:"speed,omitempty"`
	Heading     *float64    `json:"heading,omitempty"`
	Altitude    *float64    `json:"altitude,omitempty"`
	SessionStatus string    `json:"sessionStatus"`           // Session status: "active", "inactive", etc.
	UserStatus  *UserStatus `json:"status,omitempty"`        // User's custom status with emoji and message
}

// Trip model matching Laravel trips table
type Trip struct {
	ID                   uint                   `json:"id" gorm:"primaryKey"`
	SessionID            string                 `json:"session_id" gorm:"unique"`
	UserID               *uint                  `json:"user_id"`
	UserType             string                 `json:"user_type" gorm:"default:authenticated"`
	TrainID              uint                   `json:"train_id"`
	TrainName            string                 `json:"train_name"`
	TrainNumber          string                 `json:"train_number"`
	TrainRelation        *string                `json:"train_relation"`
	TotalDistanceKm      float64                `json:"total_distance_km"`
	MaxSpeedKmh          float64                `json:"max_speed_kmh"`
	AvgSpeedKmh          float64                `json:"avg_speed_kmh"`
	MaxElevationM        int                    `json:"max_elevation_m"`
	MinElevationM        int                    `json:"min_elevation_m"`
	ElevationGainM       int                    `json:"elevation_gain_m"`
	DurationSeconds      int                    `json:"duration_seconds"`
	StartLatitude        float64                `json:"start_latitude"`
	StartLongitude       float64                `json:"start_longitude"`
	EndLatitude          float64                `json:"end_latitude"`
	EndLongitude         float64                `json:"end_longitude"`
	MaxSpeedLat          *float64               `json:"max_speed_lat"`
	MaxSpeedLng          *float64               `json:"max_speed_lng"`
	MaxElevationLat      *float64               `json:"max_elevation_lat"`
	MaxElevationLng      *float64               `json:"max_elevation_lng"`
	TrackingData         interface{}            `json:"tracking_data" gorm:"type:json"`
	RouteCoordinates     interface{}            `json:"route_coordinates" gorm:"type:json"`
	FromStationID        *uint                  `json:"from_station_id"`
	FromStationName      *string                `json:"from_station_name"`
	ToStationID          *uint                  `json:"to_station_id"`
	ToStationName        *string                `json:"to_station_name"`
	StartedAt            time.Time              `json:"started_at"`
	CompletedAt          time.Time              `json:"completed_at"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

func (PersonalAccessToken) TableName() string {
	return "personal_access_tokens"
}

func (Trip) TableName() string {
	return "trips"
}

// LiveTrackingSession replaces Redis session tracking
type LiveTrackingSession struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	SessionID     string    `json:"session_id" gorm:"uniqueIndex"`
	UserID        uint      `json:"user_id" gorm:"index"`
	UserType      string    `json:"user_type" gorm:"default:authenticated"`
	ClientType    string    `json:"client_type" gorm:"default:mobile"`
	TrainID       uint      `json:"train_id"`
	TrainNumber   string    `json:"train_number"`
	FilePath      string    `json:"file_path"`
	StartedAt     time.Time `json:"started_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	AppState      *string   `json:"app_state"`
	Status        string    `json:"status" gorm:"default:active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	
	// Relationship
	User          *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

func (LiveTrackingSession) TableName() string {
	return "live_tracking_sessions"
}

// Station model matching Laravel stations table
type Station struct {
	StationID   uint     `json:"station_id" gorm:"primaryKey"`
	StationCode string   `json:"station_code"`
	StationName string   `json:"station_name"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Platforms   []Platform `json:"platforms" gorm:"foreignKey:StationID"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Station) TableName() string {
	return "stations"
}

// LaravelSession represents Laravel session data (for web users)
type LaravelSession struct {
	ID           string    `json:"id" gorm:"primaryKey;column:id"`
	UserID       *uint     `json:"user_id" gorm:"column:user_id"`
	IPAddress    *string   `json:"ip_address" gorm:"column:ip_address"`
	UserAgent    *string   `json:"user_agent" gorm:"column:user_agent"`
	Payload      string    `json:"payload" gorm:"column:payload"`
	LastActivity int64     `json:"last_activity" gorm:"column:last_activity"`
	CreatedAt    *time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    *time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (LaravelSession) TableName() string {
	return "sessions"
}

// Platform model matching Laravel platforms table
type Platform struct {
	ID                   uint    `json:"id" gorm:"primaryKey"`
	StationID            uint    `json:"station_id"`
	PlatformName         string  `json:"platform_name"`
	PlatformNumber       *string `json:"platform_number"`
	GeojsonCoordinates   *string `json:"geojson_coordinates"`
	Properties           *string `json:"properties"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (Platform) TableName() string {
	return "platforms"
}

// Train model matching Laravel trains table
type Train struct {
	TrainID         uint    `json:"train_id" gorm:"primaryKey"`
	TrainNumber     string  `json:"train_number"`
	TrainName       string  `json:"train_name"`
	Relation        *string `json:"relation"`
	TrainType       *string `json:"train_type"`
	Description     *string `json:"description"`
	MaximumSpeed    *int    `json:"maximum_speed"`
	Region          *string `json:"region"`
	IsActive        bool    `json:"is_active" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Train) TableName() string {
	return "trains"
}

// ScheduleDetail model matching Laravel schedule_details table
type ScheduleDetail struct {
	ScheduleDetailID uint      `json:"schedule_detail_id" gorm:"primaryKey"`
	TrainID          uint      `json:"train_id"`
	StationID        uint      `json:"station_id"`
	StopSequence     int       `json:"stop_sequence"`
	ArrivalTime      *string   `json:"arrival_time"`
	DepartureTime    *string   `json:"departure_time"`
	IsPassThrough    bool      `json:"is_pass_through" gorm:"default:false"`
	Remarks          *string   `json:"remarks"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Relationships
	Train   Train   `json:"train,omitempty" gorm:"foreignKey:TrainID"`
	Station Station `json:"station,omitempty" gorm:"foreignKey:StationID"`
}

func (ScheduleDetail) TableName() string {
	return "schedule_details"
}

// OperationalRoute model matching Laravel operational_routes table
type OperationalRoute struct {
	ID                      uint         `json:"id" gorm:"primaryKey"`
	OperationalRouteCode    *string      `json:"operational_route_code"`
	Name                    string       `json:"name"`
	Description             *string      `json:"description"`
	StartStationID          *uint        `json:"start_station_id"`
	EndStationID            *uint        `json:"end_station_id"`
	Status                  string       `json:"status" gorm:"default:active"`
	Operator                *string      `json:"operator"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
	// Relationships
	StartStation            *Station     `json:"start_station,omitempty" gorm:"foreignKey:StartStationID"`
	EndStation              *Station     `json:"end_station,omitempty" gorm:"foreignKey:EndStationID"`
	RailwayLines            []RailwayLine `json:"railway_lines,omitempty" gorm:"many2many:operational_route_railway_line;"`
}

func (OperationalRoute) TableName() string {
	return "operational_routes"
}

// RailwayLine model matching Laravel railway_lines table
type RailwayLine struct {
	ID                 uint                 `json:"id" gorm:"primaryKey"`
	Name               string               `json:"name"`
	OSMId              *string              `json:"osm_id"`
	RailwayType        *string              `json:"railway_type"`
	Geometry           *RailwayLineGeometry `json:"geometry,omitempty" gorm:"type:json"`
	Electrification    bool                 `json:"electrification" gorm:"default:false"`
	TrainTypesAllowed  *string              `json:"train_types_allowed"`
	FromStationID      *uint                `json:"from_station_id"`
	ToStationID        *uint                `json:"to_station_id"`
	SpeedLimit         *int                 `json:"speed_limit"`
	Notes              *string              `json:"notes"`
	Layer              *string              `json:"layer"`
	OperatorType       *string              `json:"operator_type"`
	SourceData         *string              `json:"source_data"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	// Relationships
	FromStation        *Station             `json:"from_station,omitempty" gorm:"foreignKey:FromStationID"`
	ToStation          *Station             `json:"to_station,omitempty" gorm:"foreignKey:ToStationID"`
	// Pivot data for many-to-many relationship with operational routes
	Pivot              *OperationalRoutePivot `json:"pivot,omitempty" gorm:"-"`
}

// OperationalRoutePivot represents the pivot table data
type OperationalRoutePivot struct {
	OperationalRouteID uint `json:"operational_route_id"`
	RailwayLineID      uint `json:"railway_line_id"`
	Sequence           *int `json:"sequence,omitempty"`
	IsReversed         *bool `json:"is_reversed,omitempty"`
}

func (RailwayLine) TableName() string {
	return "railway_lines"
}

// RailwayLineGeometry represents the geometry field in railway_lines
type RailwayLineGeometry struct {
	Type        string          `json:"type"`
	Coordinates [][]float64     `json:"coordinates"`
}

// Value implements the driver.Valuer interface for database storage
func (g RailwayLineGeometry) Value() (driver.Value, error) {
	return json.Marshal(g)
}

// Scan implements the sql.Scanner interface for database retrieval
func (g *RailwayLineGeometry) Scan(value interface{}) error {
	if value == nil {
		*g = RailwayLineGeometry{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan non-string value into RailwayLineGeometry")
	}

	return json.Unmarshal(bytes, g)
}