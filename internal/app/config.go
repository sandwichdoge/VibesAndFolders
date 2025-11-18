package app

import (
	"encoding/json"
	"io"
	"log"

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

var GlobalConfig *Config

func LoadConfig(a fyne.App) {
	GlobalConfig = &Config{}

	// Get config URI from app's storage root
	rootURI := a.Storage().RootURI()
	configURI, err := storage.Child(rootURI, configFileName)
	if err != nil {
		log.Printf("Error creating config URI: %v. Using defaults.", err)
		loadDefaults()
		return
	}

	exists, err := storage.Exists(configURI)
	if err != nil {
		log.Printf("Error checking config existence: %v. Using defaults.", err)
		loadDefaults()
		return
	}

	if !exists {
		log.Println("No config file found. Creating with defaults.")
		loadDefaults()
		SaveConfig(a) // Save defaults to create the file
		return
	}

	// Read config file
	rc, err := storage.Reader(configURI)
	if err != nil {
		log.Printf("Error opening config file: %v. Using defaults.", err)
		loadDefaults()
		return
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		log.Printf("Error reading config file: %v. Using defaults.", err)
		loadDefaults()
		return
	}

	if err := json.Unmarshal(data, GlobalConfig); err != nil {
		log.Printf("Error parsing config JSON: %v. Using defaults.", err)
		loadDefaults()
		return
	}

	log.Println("Configuration loaded successfully.")
}

func SaveConfig(a fyne.App) {
	data, err := json.MarshalIndent(GlobalConfig, "", "  ")
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}

	rootURI := a.Storage().RootURI()
	configURI, err := storage.Child(rootURI, configFileName)
	if err != nil {
		log.Printf("Error creating config URI for saving: %v", err)
		return
	}

	// Write config file (creates if doesn't exist)
	wc, err := storage.Writer(configURI)
	if err != nil {
		log.Printf("Error opening config file for writing: %v", err)
		return
	}
	defer wc.Close()

	if _, err := wc.Write(data); err != nil {
		log.Printf("Error writing config file: %v", err)
		return
	}

	log.Println("Configuration saved.")
}

func loadDefaults() {
	GlobalConfig.Endpoint = defaultEndpoint
	GlobalConfig.APIKey = DefaultAPIKey
	GlobalConfig.Model = defaultModel
}
