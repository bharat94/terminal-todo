package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharat94/terminal-todo/fsutil"

	"github.com/bharat94/terminal-todo/lock"
)

type ProjectConfig struct {
	DefaultTTL      string  `json:"default_ttl"`
	DefaultPriority float32 `json:"default_priority"`
	DefaultCapCaps  string  `json:"default_caps"`
}

func configPath() string {
	return filepath.Join(projectRoot, ".terminal-todo", "config.json")
}

func defaultConfig() *ProjectConfig {
	return &ProjectConfig{
		DefaultTTL:      "15m",
		DefaultPriority: 0.5,
	}
}

func loadConfig() (*ProjectConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}
	var cfg ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if ttl, err := time.ParseDuration(cfg.DefaultTTL); err != nil || ttl <= 0 {
		return nil, fmt.Errorf("invalid config: default_ttl must be a positive duration")
	}
	if !validPriority32(cfg.DefaultPriority) {
		return nil, fmt.Errorf("invalid config: default_priority must be between 0 and 1")
	}
	return &cfg, nil
}

func saveConfig(cfg *ProjectConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(configPath()), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, configPath()); err != nil {
		return err
	}
	return fsutil.SyncDir(filepath.Dir(configPath()))
}

func updateConfig(mutate func(*ProjectConfig) error) error {
	lk, err := lock.Open(configPath())
	if err != nil {
		return err
	}
	defer lk.Close()
	if err := lk.Acquire(lock.Write); err != nil {
		return err
	}
	defer lk.Release()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	return saveConfig(cfg)
}

func parseDefaultTTL(cfg *ProjectConfig) time.Duration {
	d, err := time.ParseDuration(cfg.DefaultTTL)
	if err != nil || d <= 0 {
		return 15 * time.Minute
	}
	return d
}
