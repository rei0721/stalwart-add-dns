package dnstxt

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"ddnsjx/internal/config"
)

type ZoneOptions struct {
	DefaultTTL uint64
}

func RenderZone(domain string, records []config.RawRecord, opt ZoneOptions) (string, []Issue, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" {
		return "", nil, fmt.Errorf("domain is empty")
	}

	var (
		b      strings.Builder
		issues []Issue
	)

	b.WriteString("$ORIGIN ")
	b.WriteString(domain)
	b.WriteString(".\n")
	if opt.DefaultTTL == 0 {
		opt.DefaultTTL = 300
	}
	b.WriteString("$TTL ")
	b.WriteString(strconv.FormatUint(opt.DefaultTTL, 10))
	b.WriteString("\n\n")

	for i, rr := range records {
		lineNo := i + 1
		t := strings.ToUpper(strings.TrimSpace(rr.Type))
		name := strings.TrimSpace(rr.Name)
		if name == "" {
			issues = append(issues, Issue{Line: lineNo, Level: "warn", Message: "empty name"})
			continue
		}
		name = strings.TrimSuffix(name, ".")

		owner := toRelativeOwner(domain, name)
		if owner == "" {
			owner = name + "."
		}

		var ttlStr string
		if rr.TTL != nil && *rr.TTL > 0 {
			ttlStr = strconv.FormatUint(*rr.TTL, 10)
		}

		rdata, warn := toZoneRData(t, rr.Contents)
		if warn != "" {
			issues = append(issues, Issue{Line: lineNo, Level: "warn", Message: warn})
		}
		if rdata == "" {
			continue
		}

		b.WriteString(owner)
		if ttlStr != "" {
			b.WriteString(" ")
			b.WriteString(ttlStr)
		}
		b.WriteString(" IN ")
		b.WriteString(t)
		b.WriteString(" ")
		b.WriteString(rdata)
		b.WriteString("\n")
	}

	return b.String(), issues, nil
}

func toRelativeOwner(domain, name string) string {
	if name == domain {
		return "@"
	}
	suffix := "." + domain
	if strings.HasSuffix(name, suffix) {
		sub := strings.TrimSuffix(name, suffix)
		if sub == "" {
			return "@"
		}
		return sub
	}
	return ""
}

func toZoneRData(t, contents string) (string, string) {
	contents = strings.TrimSpace(contents)
	if contents == "" {
		return "", "empty contents"
	}

	switch t {
	case "TXT":
		return quoteTXT(contents), ""
	case "CNAME":
		return ensureFQDN(contents), ""
	case "MX":
		f := strings.Fields(contents)
		if len(f) < 2 {
			return "", fmt.Sprintf("MX contents expects: \"<priority> <exchange>\", got %q", contents)
		}
		ex := ensureFQDN(f[1])
		return f[0] + " " + ex, ""
	case "SRV":
		f := strings.Fields(contents)
		if len(f) < 4 {
			return "", fmt.Sprintf("SRV contents expects: \"<priority> <weight> <port> <target>\", got %q", contents)
		}
		target := ensureFQDN(f[3])
		return strings.Join([]string{f[0], f[1], f[2], target}, " "), ""
	default:
		return contents, ""
	}
}

func ensureFQDN(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.HasSuffix(v, ".") {
		return v
	}
	if net.ParseIP(v) != nil {
		return v
	}
	if strings.Contains(v, ".") {
		return v + "."
	}
	return v
}

func quoteTXT(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "\"\""
	}
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "\"", "\\\"")

	const maxChunk = 200
	if len(v) <= maxChunk {
		return "\"" + v + "\""
	}

	var parts []string
	for len(v) > 0 {
		n := maxChunk
		if len(v) < n {
			n = len(v)
		}
		parts = append(parts, "\""+v[:n]+"\"")
		v = v[n:]
	}
	return "(" + strings.Join(parts, " ") + ")"
}
