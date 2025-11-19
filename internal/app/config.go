package app

import (
	"encoding/json"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

const (
	configFileName = "config.json"

	// Default values
	defaultEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	DefaultAPIKey   = "YOUR_API_KEY_HERE"
	defaultModel    = "moonshotai/kimi-k2-0905"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

// LoadConfig loads configuration from app storage
func LoadConfig(a fyne.App, logger *Logger) *Config {
	config := &Config{}

	// Get config URI from app's storage root
	rootURI := a.Storage().RootURI()
	configURI, err := storage.Child(rootURI, configFileName)
	if err != nil {
		logger.Info("Error creating config URI: %v. Using defaults.", err)
		loadDefaults(config)
		return config
	}

	exists, err := storage.Exists(configURI)
	if err != nil {
		logger.Info("Error checking config existence: %v. Using defaults.", err)
		loadDefaults(config)
		return config
	}

	if !exists {
		logger.Info("No config file found. Creating with defaults.")
		loadDefaults(config)
		SaveConfig(a, config, logger)
		return config
	}

	// Read config file
	rc, err := storage.Reader(configURI)
	if err != nil {
		logger.Info("Error opening config file: %v. Using defaults.", err)
		loadDefaults(config)
		return config
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		logger.Info("Error reading config file: %v. Using defaults.", err)
		loadDefaults(config)
		return config
	}

	if err := json.Unmarshal(data, config); err != nil {
		logger.Info("Error parsing config JSON: %v. Using defaults.", err)
		loadDefaults(config)
		return config
	}

	logger.Info("Configuration loaded successfully.")
	return config
}

// SaveConfig saves configuration to app storage
func SaveConfig(a fyne.App, config *Config, logger *Logger) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logger.Info("Error marshaling config: %v", err)
		return
	}

	rootURI := a.Storage().RootURI()
	configURI, err := storage.Child(rootURI, configFileName)
	if err != nil {
		logger.Info("Error creating config URI for saving: %v", err)
		return
	}

	// Write config file (creates if doesn't exist)
	wc, err := storage.Writer(configURI)
	if err != nil {
		logger.Info("Error opening config file for writing: %v", err)
		return
	}
	defer wc.Close()

	if _, err := wc.Write(data); err != nil {
		logger.Info("Error writing config file: %v", err)
		return
	}

	logger.Info("Configuration saved.")
}

func loadDefaults(config *Config) {
	config.Endpoint = defaultEndpoint
	config.APIKey = DefaultAPIKey
	config.Model = defaultModel
}
