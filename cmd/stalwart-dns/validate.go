package main

import (
	"fmt"
	"sort"
	"strings"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/dnspodclient"
)

func validateOrFilterPlanForDNSPod(plan dns.Plan, skipUnsupported bool) (dns.Plan, error) {
	var (
		filtered    []dns.Record
		unsupported = make(map[string]struct{}, 4)
	)

	for _, r := range plan.Records {
		t := strings.ToUpper(strings.TrimSpace(r.Type))
		if dnspodclient.IsSupportedRecordType(t) {
			filtered = append(filtered, r)
			continue
		}
		unsupported[t] = struct{}{}
		if skipUnsupported {
			continue
		}
	}

	if len(unsupported) > 0 && !skipUnsupported {
		var types []string
		for t := range unsupported {
			types = append(types, t)
		}
		sort.Strings(types)
		return dns.Plan{}, fmt.Errorf("unsupported DNSPod record type(s): %s (use --skip-unsupported to ignore them)", strings.Join(types, ", "))
	}

	plan.Records = filtered
	return plan, nil
}
