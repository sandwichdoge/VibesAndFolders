package app

import (
	"errors"
	"os"
	"strings"
)

var (
	ErrEmptyDirectory      = errors.New("directory path cannot be empty")
	ErrEmptyPrompt         = errors.New("organization instructions cannot be empty")
	ErrEmptyEndpoint       = errors.New("endpoint field cannot be empty")
	ErrInvalidConfig       = errors.New("please configure your AI Endpoint and API Key first")
	ErrInvalidDepth        = errors.New("invalid depth selected")
	ErrSourceNotExist      = errors.New("source file does not exist")
	ErrDestinationExists   = errors.New("destination already exists")
	ErrCannotCreateDir     = errors.New("could not create directory")
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateDirectory(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrEmptyDirectory
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return err
	}
	return nil
}

func (v *Validator) ValidatePrompt(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return ErrEmptyPrompt
	}
	return nil
}

func (v *Validator) ValidateConfig(config *Config) error {
	if strings.TrimSpace(config.Endpoint) == "" {
		return ErrEmptyEndpoint
	}
	if config.APIKey == "" || config.APIKey == DefaultAPIKey {
		return ErrInvalidConfig
	}
	return nil
}

func (v *Validator) ValidateFileOperation(op FileOperation) error {
	// Use Lstat instead of Stat to handle symlinks properly
	// Lstat doesn't follow symlinks, so it will succeed even if the symlink target doesn't exist
	if _, err := os.Lstat(op.From); os.IsNotExist(err) {
		return ErrSourceNotExist
	}
	if _, err := os.Lstat(op.To); err == nil {
		return ErrDestinationExists
	}
	return nil
}
