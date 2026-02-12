package dnstxt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"ddnsjx/internal/config"
)

type Issue struct {
	Line    int
	Level   string
	Message string
}

func LoadFile(path string) ([]config.RawRecord, []Issue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) ([]config.RawRecord, []Issue, error) {
	var (
		records []config.RawRecord
		issues  []Issue
	)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var header map[string]int

	for lineNo := 1; sc.Scan(); lineNo++ {
		raw := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			continue
		}

		cols, hadTabs := splitColumns(raw)
		if len(cols) == 0 {
			continue
		}

		if header == nil && looksLikeHeader(cols) {
			header = buildHeader(cols)
			continue
		}

		rec, recIssues, ok := parseRecordLine(lineNo, cols, hadTabs, header)
		issues = append(issues, recIssues...)
		if ok {
			records = append(records, rec)
		}
	}

	if err := sc.Err(); err != nil {
		return nil, issues, err
	}
	return records, issues, nil
}

func splitColumns(line string) ([]string, bool) {
	if strings.Contains(line, "\t") {
		parts := strings.Split(line, "\t")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts, true
	}
	return strings.Fields(line), false
}

func looksLikeHeader(cols []string) bool {
	if len(cols) < 3 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cols[0]), "type") &&
		strings.EqualFold(strings.TrimSpace(cols[1]), "name") &&
		(strings.EqualFold(strings.TrimSpace(cols[2]), "contents") ||
			strings.EqualFold(strings.TrimSpace(cols[2]), "content") ||
			strings.EqualFold(strings.TrimSpace(cols[2]), "value"))
}

func buildHeader(cols []string) map[string]int {
	h := make(map[string]int, len(cols))
	for i, c := range cols {
		key := strings.ToLower(strings.TrimSpace(c))
		h[key] = i
	}
	return h
}

func parseRecordLine(lineNo int, cols []string, hadTabs bool, header map[string]int) (config.RawRecord, []Issue, bool) {
	var issues []Issue

	get := func(key string) (string, bool) {
		if header == nil {
			return "", false
		}
		i, ok := header[key]
		if !ok || i < 0 || i >= len(cols) {
			return "", false
		}
		return strings.TrimSpace(cols[i]), true
	}

	var (
		t       string
		name    string
		content string
		ttlRaw  string
		remark  string
	)

	if header != nil {
		var ok bool
		t, ok = get("type")
		if !ok {
			issues = append(issues, Issue{Line: lineNo, Level: "error", Message: "missing column: type"})
			return config.RawRecord{}, issues, false
		}
		name, ok = get("name")
		if !ok {
			issues = append(issues, Issue{Line: lineNo, Level: "error", Message: "missing column: name"})
			return config.RawRecord{}, issues, false
		}
		content, ok = get("contents")
		if !ok {
			content, _ = get("content")
		}
		if content == "" {
			content, _ = get("value")
		}
		if content == "" {
			issues = append(issues, Issue{Line: lineNo, Level: "error", Message: "missing column: contents"})
			return config.RawRecord{}, issues, false
		}
		ttlRaw, _ = get("ttl")
		remark, _ = get("remark")
	} else {
		if len(cols) < 3 {
			issues = append(issues, Issue{Line: lineNo, Level: "error", Message: "expected at least 3 columns: Type Name Contents"})
			return config.RawRecord{}, issues, false
		}
		t = cols[0]
		name = cols[1]
		if hadTabs {
			content = strings.Join(cols[2:], "\t")
		} else {
			content = strings.Join(cols[2:], " ")
		}
	}

	t = strings.ToUpper(strings.TrimSpace(t))
	name = strings.TrimSpace(name)
	content = strings.TrimSpace(content)

	if t == "" || name == "" || content == "" {
		issues = append(issues, Issue{Line: lineNo, Level: "error", Message: "type/name/contents must be non-empty"})
		return config.RawRecord{}, issues, false
	}

	name = maybeEnsureTrailingDot(name)

	if t == "TXT" {
		if len(content) >= 2 && strings.HasPrefix(content, "\"") && strings.HasSuffix(content, "\"") {
			content = strings.TrimSuffix(strings.TrimPrefix(content, "\""), "\"")
		}
	}

	rec := config.RawRecord{
		Type:     t,
		Name:     name,
		Contents: content,
		Remark:   strings.TrimSpace(remark),
	}

	if ttlRaw != "" {
		v, err := strconv.ParseUint(strings.TrimSpace(ttlRaw), 10, 64)
		if err != nil {
			issues = append(issues, Issue{Line: lineNo, Level: "warn", Message: fmt.Sprintf("invalid ttl %q: %v", ttlRaw, err)})
		} else {
			rec.TTL = &v
		}
	}

	switch t {
	case "MX":
		rr, warn := normalizeMXContents(rec.Contents)
		if warn != "" {
			issues = append(issues, Issue{Line: lineNo, Level: "warn", Message: warn})
		}
		if rr != nil {
			rec.Contents = rr.Contents
			rec.Parsed = rr.Parsed
		}
	case "SRV":
		rr, warn := normalizeSRVContents(rec.Contents)
		if warn != "" {
			issues = append(issues, Issue{Line: lineNo, Level: "warn", Message: warn})
		}
		if rr != nil {
			rec.Contents = rr.Contents
			rec.Parsed = rr.Parsed
		}
	case "CNAME":
		rec.Contents = maybeEnsureTrailingDot(rec.Contents)
	}

	return rec, issues, true
}

func maybeEnsureTrailingDot(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if s == "@" {
		return s
	}
	if strings.HasSuffix(s, ".") {
		return s
	}
	if strings.Contains(s, ".") {
		return s + "."
	}
	return s
}

func normalizeMXContents(contents string) (*config.RawRecord, string) {
	f := strings.Fields(contents)
	if len(f) < 2 {
		return nil, fmt.Sprintf("MX contents expects: \"<priority> <exchange>\", got %q", contents)
	}

	prio, err := strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return nil, fmt.Sprintf("MX priority parse failed for %q: %v", f[0], err)
	}

	exchange := maybeEnsureTrailingDot(f[1])
	normalized := fmt.Sprintf("%d %s", prio, exchange)

	return &config.RawRecord{
		Contents: normalized,
		Parsed: &config.RawParsed{
			Priority: &prio,
			Exchange: &exchange,
		},
	}, ""
}

func normalizeSRVContents(contents string) (*config.RawRecord, string) {
	f := strings.Fields(contents)
	if len(f) < 4 {
		return nil, fmt.Sprintf("SRV contents expects: \"<priority> <weight> <port> <target>\", got %q", contents)
	}

	priority, err := strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return nil, fmt.Sprintf("SRV priority parse failed for %q: %v", f[0], err)
	}
	weight, err := strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return nil, fmt.Sprintf("SRV weight parse failed for %q: %v", f[1], err)
	}
	port, err := strconv.ParseUint(f[2], 10, 64)
	if err != nil {
		return nil, fmt.Sprintf("SRV port parse failed for %q: %v", f[2], err)
	}
	target := maybeEnsureTrailingDot(f[3])

	normalized := fmt.Sprintf("%d %d %d %s", priority, weight, port, target)
	return &config.RawRecord{
		Contents: normalized,
		Parsed: &config.RawParsed{
			Priority: &priority,
			Weight:   &weight,
			Port:     &port,
			Target:   &target,
		},
	}, ""
}
