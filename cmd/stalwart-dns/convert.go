package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"ddnsjx/internal/config"
	"ddnsjx/internal/dnstxt"
)

func runConvert(args []string) int {
	fs := flag.NewFlagSet("stalwart-dns convert", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		inputPath  = fs.String("input", "dns.txt", "path to dns.txt (TSV)")
		outputPath = fs.String("output", "config.json", "path to output config.json (use - for stdout)")
		zonePath   = fs.String("zone", "", "path to output zone file (optional, use - for stdout)")
		domain     = fs.String("domain", "", "domain (empty: infer from records)")
		pretty     = fs.Bool("pretty", true, "pretty-print JSON")
		force      = fs.Bool("force", false, "write outputs even if issues exist")
		defaultTTL = fs.Uint64("default-ttl", 300, "default TTL for zone output (ignored if --zone is empty)")
		replace    = fs.String("replace-target", "", "replace value/target in records, format: old=new")
	)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	records, issues, err := dnstxt.LoadFile(*inputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read dns.txt:", err.Error())
		return 1
	}

	if *replace != "" {
		applyReplacements(records, *replace)
	}

	printIssues(issues)

	var hasError bool
	for _, is := range issues {
		if strings.EqualFold(is.Level, "error") {
			hasError = true
			break
		}
	}
	if hasError && !*force {
		fmt.Fprintln(os.Stderr, "convert aborted due to errors (use --force to write anyway)")
		return 1
	}

	resolvedDomain := strings.TrimSuffix(strings.TrimSpace(*domain), ".")
	if resolvedDomain == "" {
		resolvedDomain = config.InferDomain(records)
	}
	if resolvedDomain == "" && !*force {
		fmt.Fprintln(os.Stderr, "domain is required (flag --domain) or inferable from records")
		return 1
	}

	if resolvedDomain != "" {
		if _, err := config.BuildPlan(resolvedDomain, "默认", records); err != nil {
			fmt.Fprintln(os.Stderr, "config self-check failed:", err.Error())
			if !*force {
				return 1
			}
		}
	}

	countByType := make(map[string]int, 8)
	for _, r := range records {
		countByType[strings.ToUpper(strings.TrimSpace(r.Type))]++
	}
	var keys []string
	for k := range countByType {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "records[%s]=%d\n", k, countByType[k])
	}

	cfg := config.FileConfig{Records: records}
	var out []byte
	if *pretty {
		out, err = json.MarshalIndent(cfg, "", "  ")
	} else {
		out, err = json.Marshal(cfg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode config json:", err.Error())
		return 1
	}
	out = append(out, '\n')
	if err := writeFileOrStdout(*outputPath, out); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	if strings.TrimSpace(*zonePath) != "" {
		zone, zIssues, err := dnstxt.RenderZone(resolvedDomain, records, dnstxt.ZoneOptions{DefaultTTL: *defaultTTL})
		if err != nil {
			fmt.Fprintln(os.Stderr, "render zone:", err.Error())
			if !*force {
				return 1
			}
		}
		printIssues(zIssues)
		if err := writeFileOrStdout(*zonePath, []byte(zone)); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
	}

	return 0
}

func printIssues(issues []dnstxt.Issue) {
	for _, is := range issues {
		if is.Line > 0 {
			fmt.Fprintf(os.Stderr, "%s: line %d: %s\n", strings.ToLower(is.Level), is.Line, is.Message)
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", strings.ToLower(is.Level), is.Message)
	}
}

func writeFileOrStdout(path string, data []byte) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path is empty")
	}
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
