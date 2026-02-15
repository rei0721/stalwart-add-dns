package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ddnsjx/internal/config"
	"ddnsjx/internal/dnstxt"
)

func initConfigFromDNSTxt(dnsTxtPath string, configPath string, force bool, replaceTarget string) error {
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

	if replaceTarget != "" {
		applyReplacements(records, replaceTarget)
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

func applyReplacements(records []config.RawRecord, replaceTarget string) {
	parts := strings.SplitN(replaceTarget, "=", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "warning: invalid replace format %q (expected old=new), ignoring\n", replaceTarget)
		return
	}
	oldStr, newStr := parts[0], parts[1]
	if oldStr == "" {
		return
	}

	for i := range records {
		r := &records[i]
		// Replace in Contents
		if strings.Contains(r.Contents, oldStr) {
			r.Contents = strings.ReplaceAll(r.Contents, oldStr, newStr)
		}
		// Replace in Parsed fields
		if r.Parsed != nil {
			if r.Parsed.Exchange != nil && strings.Contains(*r.Parsed.Exchange, oldStr) {
				newVal := strings.ReplaceAll(*r.Parsed.Exchange, oldStr, newStr)
				r.Parsed.Exchange = &newVal
			}
			if r.Parsed.Target != nil && strings.Contains(*r.Parsed.Target, oldStr) {
				newVal := strings.ReplaceAll(*r.Parsed.Target, oldStr, newStr)
				r.Parsed.Target = &newVal
			}
		}
	}
}
