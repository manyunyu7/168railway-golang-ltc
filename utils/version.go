package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

type VersionConfig struct {
	CurrentVersion string `json:"current_version"`
	MinimumVersion string `json:"minimum_version"`
	ForceUpdate    bool   `json:"force_update"`
	UpdateMessage  string `json:"update_message"`
	DownloadURL    string `json:"download_url"`
	LastUpdated    string `json:"last_updated"`
}

var (
	versionConfig *VersionConfig
	configPath    = "version-config.json"
)

// LoadVersionConfig loads version configuration from JSON file
func LoadVersionConfig() (*VersionConfig, error) {
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		defaultConfig := &VersionConfig{
			CurrentVersion: "1.2.0",
			MinimumVersion: "1.1.0",
			ForceUpdate:    false,
			UpdateMessage:  "A new version is available with bug fixes and improvements!",
			DownloadURL:    "https://github.com/manyunyu7/168railway-golang-ltc/releases",
			LastUpdated:    time.Now().Format(time.RFC3339),
		}
		
		if err := SaveVersionConfig(defaultConfig); err != nil {
			return nil, fmt.Errorf("failed to create default config: %v", err)
		}
		
		versionConfig = defaultConfig
		return defaultConfig, nil
	}
	
	// Read existing file
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read version config: %v", err)
	}
	
	var config VersionConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse version config: %v", err)
	}
	
	versionConfig = &config
	return &config, nil
}

// SaveVersionConfig saves version configuration to JSON file
func SaveVersionConfig(config *VersionConfig) error {
	config.LastUpdated = time.Now().Format(time.RFC3339)
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	
	if err := ioutil.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}
	
	versionConfig = config
	return nil
}

// GetVersionConfig returns the current version configuration
func GetVersionConfig() *VersionConfig {
	if versionConfig == nil {
		config, err := LoadVersionConfig()
		if err != nil {
			// Return default config if loading fails
			return &VersionConfig{
				CurrentVersion: "1.2.0",
				MinimumVersion: "1.1.0",
				ForceUpdate:    false,
				UpdateMessage:  "A new version is available with bug fixes and improvements!",
				DownloadURL:    "https://github.com/manyunyu7/168railway-golang-ltc/releases",
				LastUpdated:    time.Now().Format(time.RFC3339),
			}
		}
		return config
	}
	return versionConfig
}

// ReloadVersionConfig forces reload of the version configuration from file
func ReloadVersionConfig() (*VersionConfig, error) {
	versionConfig = nil // Clear cached config
	return LoadVersionConfig()
}