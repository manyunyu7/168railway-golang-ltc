package models

import (
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

type Passenger struct {
	UserID      uint    `json:"userId"`
	UserType    string  `json:"userType"`
	ClientType  string  `json:"clientType"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Timestamp   int64   `json:"timestamp"`
	SessionID   string  `json:"sessionId"`
	Accuracy    *float64 `json:"accuracy,omitempty"`
	Speed       *float64 `json:"speed,omitempty"`
	Heading     *float64 `json:"heading,omitempty"`
	Altitude    *float64 `json:"altitude,omitempty"`
	Status      string  `json:"status"`
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
}

func (LiveTrackingSession) TableName() string {
	return "live_tracking_sessions"
}