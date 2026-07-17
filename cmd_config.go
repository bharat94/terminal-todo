package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func cmdConfig(args []string) {
	if len(args) == 0 {
		showConfig()
		return
	}

	if len(args) == 1 && !strings.Contains(args[0], "=") {
		showConfigValue(args[0])
		return
	}

	if err := updateConfig(func(cfg *ProjectConfig) error {
		for _, arg := range args {
			key, value, ok := strings.Cut(arg, "=")
			if !ok {
				return fmt.Errorf("invalid format %q, use key=value", arg)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			switch key {
			case "default_ttl":
				d, err := time.ParseDuration(value)
				if err != nil || d <= 0 {
					return fmt.Errorf("default_ttl must be a positive duration (e.g. 15m, 1h)")
				}
				cfg.DefaultTTL = value
			case "default_priority":
				p, err := strconv.ParseFloat(value, 32)
				if err != nil || !validPriority(p) {
					return fmt.Errorf("default_priority must be between 0 and 1")
				}
				cfg.DefaultPriority = float32(p)
			case "default_caps":
				cfg.DefaultCapCaps = value
			default:
				return fmt.Errorf("unknown config key %q (valid: default_ttl, default_priority, default_caps)", key)
			}
		}
		return nil
	}); err != nil {
		fail(ErrInvalidArgs, "%v", err)
	}

	for _, arg := range args {
		fmt.Printf("Set %s\n", arg)
	}
}

func showConfig() {
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	fmt.Printf("default_ttl       = %s\n", cfg.DefaultTTL)
	fmt.Printf("default_priority  = %.2f\n", cfg.DefaultPriority)
	fmt.Printf("default_caps      = %s\n", cfg.DefaultCapCaps)
}

func showConfigValue(key string) {
	cfg, err := loadConfig()
	if err != nil {
		fail(ErrStoreCorrupted, "loading config: %v", err)
	}
	switch key {
	case "default_ttl":
		fmt.Println(cfg.DefaultTTL)
	case "default_priority":
		fmt.Printf("%.2f\n", cfg.DefaultPriority)
	case "default_caps":
		fmt.Println(cfg.DefaultCapCaps)
	default:
		fail(ErrInvalidArgs, "unknown config key %q", key)
	}
}
