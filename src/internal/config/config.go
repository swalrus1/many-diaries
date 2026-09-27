package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Storage StorageConfig `yaml:"storage"`
}

type StorageConfig struct {
	Disk DiskConfig `yaml:"disk"`
}

type DiskConfig struct {
	Root string `yaml:"root"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.Storage.Disk.Root == "" {
		return nil, errors.New("config: storage.disk.root is required")
	}
	return &cfg, nil
}
