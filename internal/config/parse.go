package config

import (
	"fmt"
	"strconv"
	"strings"
)

func parseMX(rr RawRecord) (uint64, string, error) {
	if rr.Parsed != nil && rr.Parsed.Priority != nil && rr.Parsed.Exchange != nil {
		return *rr.Parsed.Priority, *rr.Parsed.Exchange, nil
	}
	fields := strings.Fields(strings.TrimSpace(rr.Contents))
	if len(fields) < 2 {
		return 0, "", fmt.Errorf("invalid MX contents: %q", rr.Contents)
	}
	p, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid MX priority: %q", fields[0])
	}
	exchange := fields[1]
	return p, exchange, nil
}

func parseSRV(rr RawRecord) (priority, weight, port uint64, target string, err error) {
	if rr.Parsed != nil && rr.Parsed.Priority != nil && rr.Parsed.Weight != nil && rr.Parsed.Port != nil && rr.Parsed.Target != nil {
		return *rr.Parsed.Priority, *rr.Parsed.Weight, *rr.Parsed.Port, *rr.Parsed.Target, nil
	}
	fields := strings.Fields(strings.TrimSpace(rr.Contents))
	if len(fields) < 4 {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV contents: %q", rr.Contents)
	}
	p, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV priority: %q", fields[0])
	}
	w, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV weight: %q", fields[1])
	}
	po, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV port: %q", fields[2])
	}
	target = fields[3]
	return p, w, po, target, nil
}
