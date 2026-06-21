package config

import (
	"encoding/json"
	"errors"
	"os"
)

const configFile = "builder.json"

type Manager struct{}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrConfigNotFound
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (m *Manager) Save(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, append(data, '\n'), 0644)
}
