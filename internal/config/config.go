package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type FileConfig struct {
	Records []RawRecord `json:"records"`
}

func LoadFile(path string) (FileConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, fmt.Errorf("read config: %w", err)
	}
	var cfg FileConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse config json: %w", err)
	}
	return cfg, nil
}
