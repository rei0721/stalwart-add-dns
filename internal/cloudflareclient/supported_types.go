package cloudflareclient

import "strings"

func IsSupportedRecordType(t string) bool {
	t = strings.ToUpper(strings.TrimSpace(t))
	switch t {
	case "A", "AAAA", "CNAME", "MX", "TXT", "SRV", "NS", "CAA", "PTR", "NAPTR", "TLSA":
		return true
	default:
		return false
	}
}
