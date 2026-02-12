package config

import (
	"fmt"
	"strings"

	"ddnsjx/internal/dns"
)

type RawRecord struct {
	Type     string     `json:"type"`
	Name     string     `json:"name"`
	Contents string     `json:"contents"`
	Remark   string     `json:"remark,omitempty"`
	TTL      *uint64    `json:"ttl,omitempty"`
	Parsed   *RawParsed `json:"parsed,omitempty"`
}

type RawParsed struct {
	Priority *uint64 `json:"priority,omitempty"`
	Weight   *uint64 `json:"weight,omitempty"`
	Port     *uint64 `json:"port,omitempty"`
	Target   *string `json:"target,omitempty"`
	Exchange *string `json:"exchange,omitempty"`
}

func InferDomain(records []RawRecord) string {
	best := ""
	for _, r := range records {
		fqdn := strings.TrimSuffix(strings.TrimSpace(r.Name), ".")
		if fqdn == "" {
			continue
		}
		if best == "" || len(fqdn) < len(best) {
			best = fqdn
		}
	}
	return best
}

func BuildPlan(domain, recordLine string, records []RawRecord) (dns.Plan, error) {
	if domain == "" {
		return dns.Plan{}, fmt.Errorf("domain is empty")
	}
	if recordLine == "" {
		recordLine = "默认"
	}
	plan := dns.Plan{Domain: domain, RecordLine: recordLine}
	for i, rr := range records {
		rec, err := normalizeRecord(domain, rr)
		if err != nil {
			return dns.Plan{}, fmt.Errorf("record[%d]: %w", i, err)
		}
		plan.Records = append(plan.Records, rec)
	}
	return plan, nil
}

func normalizeRecord(domain string, rr RawRecord) (dns.Record, error) {
	t := strings.ToUpper(strings.TrimSpace(rr.Type))
	if t == "" {
		return dns.Record{}, fmt.Errorf("type is empty")
	}

	name := strings.TrimSpace(rr.Name)
	if name == "" {
		return dns.Record{}, fmt.Errorf("name is empty")
	}
	name = strings.TrimSuffix(name, ".")

	sub, err := toSubDomain(domain, name)
	if err != nil {
		return dns.Record{}, err
	}

	contents := strings.TrimSpace(rr.Contents)
	if contents == "" {
		return dns.Record{}, fmt.Errorf("contents is empty")
	}

	remark := strings.TrimSpace(rr.Remark)

	var (
		value    string
		priority *uint64
	)

	switch t {
	case "MX":
		p, exchange, err := parseMX(rr)
		if err != nil {
			return dns.Record{}, err
		}
		priority = &p
		value = trimTrailingDot(exchange)
	case "SRV":
		p, weight, port, target, err := parseSRV(rr)
		if err != nil {
			return dns.Record{}, err
		}
		value = fmt.Sprintf("%d %d %d %s", p, weight, port, ensureTrailingDot(target))
	case "CNAME":
		value = trimTrailingDot(contents)
	case "TXT":
		value = contents
	default:
		value = contents
	}

	return dns.Record{
		SubDomain: sub,
		Type:      t,
		Value:     value,
		Priority:  priority,
		Remark:    remark,
		TTL:       rr.TTL,
	}, nil
}

func toSubDomain(domain, name string) (string, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if domain == "" {
		return "", fmt.Errorf("domain is empty")
	}
	if name == domain {
		return "@", nil
	}
	suffix := "." + domain
	if !strings.HasSuffix(name, suffix) {
		return "", fmt.Errorf("name %q is not under domain %q", name, domain)
	}
	sub := strings.TrimSuffix(name, suffix)
	if sub == "" {
		return "@", nil
	}
	return sub, nil
}

func trimTrailingDot(s string) string {
	return strings.TrimSuffix(strings.TrimSpace(s), ".")
}

func ensureTrailingDot(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}
