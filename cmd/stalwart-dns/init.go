package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ddnsjx/internal/config"
	"ddnsjx/internal/dnstxt"
)

func initConfigFromDNSTxt(dnsTxtPath string, configPath string, force bool) error {
	dnsTxtPath = strings.TrimSpace(dnsTxtPath)
	configPath = strings.TrimSpace(configPath)
	if dnsTxtPath == "" {
		return fmt.Errorf("dns.txt path is empty")
	}
	if configPath == "" {
		return fmt.Errorf("config path is empty")
	}

	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s (use --force)", configPath)
		}
	}

	records, issues, err := dnstxt.LoadFile(dnsTxtPath)
	if err != nil {
		return fmt.Errorf("read dns.txt: %w", err)
	}
	for _, is := range issues {
		if strings.EqualFold(is.Level, "error") {
			return fmt.Errorf("dns.txt parse error at line %d: %s", is.Line, is.Message)
		}
	}

	cfg := config.FileConfig{Records: records}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config json: %w", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	return nil
}
