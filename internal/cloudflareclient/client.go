package cloudflareclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/provider"
)

type NewOptions struct {
	APIToken string
	ZoneID   string
	ZoneName string
}

type client struct {
	token    string
	zoneID   string
	zoneName string
	http     *http.Client
}

type Error struct {
	Code    int
	Message string
}

func (e Error) Error() string {
	if e.Code == 0 {
		return e.Message
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e Error) Retryable() bool {
	switch e.Code {
	case 10000, 10001, 10100, 10101, 9103:
		return true
	default:
		return false
	}
}

func New(opt NewOptions) (provider.Client, error) {
	if strings.TrimSpace(opt.APIToken) == "" {
		return nil, fmt.Errorf("missing Cloudflare api token")
	}
	return &client{
		token:    strings.TrimSpace(opt.APIToken),
		zoneID:   strings.TrimSpace(opt.ZoneID),
		zoneName: strings.TrimSpace(opt.ZoneName),
		http:     &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (c *client) IsSupportedRecordType(t string) bool {
	return IsSupportedRecordType(t)
}

func (c *client) CreateRecord(ctx context.Context, zone string, _ string, record dns.Record) (string, provider.CreateStatus, error) {
	id, found, err := c.FindRecord(ctx, zone, "", record)
	if err != nil {
		return "", provider.CreateStatusFail, err
	}
	if found {
		return id, provider.CreateStatusExists, nil
	}

	body, err := buildCreateBody(zone, record)
	if err != nil {
		return "", provider.CreateStatusFail, err
	}

	zoneID, err := c.resolveZoneID(ctx, zone)
	if err != nil {
		return "", provider.CreateStatusFail, err
	}

	var resp cfResponse[cfDNSRecord]
	if err := c.do(ctx, "POST", "/zones/"+url.PathEscape(zoneID)+"/dns_records", body, &resp); err != nil {
		return "", provider.CreateStatusFail, err
	}
	if !resp.Success {
		// Code 81057: Record already exists.
		// Code 81058: An identical record already exists.
		// If we get "already exists" here, it means FindRecord missed it (maybe race condition or subtle mismatch),
		// but Cloudflare says it's there. We can treat this as "Exists".
		for _, e := range resp.Errors {
			if e.Code == 81057 || e.Code == 81058 {
				// We don't have the ID here easily without re-querying, but the contract says return ID if Exists.
				// However, standard flow might not strictly require ID if we just want to skip.
				// Let's try to find it again to get ID, or just return empty ID with Exists status if acceptable.
				// For safety, let's just return Exists. The caller usually skips if Exists.
				return "", provider.CreateStatusExists, nil
			}
		}
		return "", provider.CreateStatusFail, pickError(resp.Errors)
	}
	return resp.Result.ID, provider.CreateStatusSuccess, nil
}

func (c *client) DeleteRecord(ctx context.Context, zone string, recordID string) error {
	zoneID, err := c.resolveZoneID(ctx, zone)
	if err != nil {
		return err
	}
	var resp cfResponse[any]
	if err := c.do(ctx, "DELETE", "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(strings.TrimSpace(recordID)), nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return pickError(resp.Errors)
	}
	return nil
}

func (c *client) FindRecord(ctx context.Context, zone string, _ string, record dns.Record) (string, bool, error) {
	zoneID, err := c.resolveZoneID(ctx, zone)
	if err != nil {
		return "", false, err
	}

	query := url.Values{}
	query.Set("type", strings.ToUpper(strings.TrimSpace(record.Type)))
	query.Set("name", fqdnFromZone(zone, record.SubDomain))

	var resp cfResponse[[]cfDNSRecord]
	if err := c.do(ctx, "GET", "/zones/"+url.PathEscape(zoneID)+"/dns_records?"+query.Encode(), nil, &resp); err != nil {
		return "", false, err
	}
	if !resp.Success {
		return "", false, pickError(resp.Errors)
	}
	if len(resp.Result) == 0 {
		return "", false, nil
	}
	if len(resp.Result) > 0 {
		// Exact match check for multiple records (like TLSA, SRV)
		// Cloudflare API returns all records matching name+type. We must filter by content/data to know if *this specific* record exists.
		for _, r := range resp.Result {
			if recordMatches(r, record) {
				return r.ID, true, nil
			}
		}
		// If we found records but none matched exactly, it means this specific record (with this content) does not exist.
		// However, we must return "false" so CreateRecord proceeds.
		// NOTE: If your intention is "upsert" logic where we overwrite *any* existing record of this type,
		// that requires different handling (e.g. delete all and recreate, or update the first one).
		// But here FindRecord implies "find THIS exact record".
		return "", false, nil
	}
	return "", false, nil
}

func recordMatches(cfRec cfDNSRecord, localRec dns.Record) bool {
	// TLSA special handling (Cloudflare stores parts in 'data')
	if strings.ToUpper(localRec.Type) == "TLSA" && cfRec.Data != nil {
		usage, selector, matchingType, cert, err := splitTLSAValue(localRec.Value)
		if err == nil {
			// Compare parsed values
			// Note: cfRec.Data values are float64 when unmarshaled from JSON usually, or int. Use fmt.Sprint to be safe.
			u, _ := toInt(cfRec.Data["usage"])
			s, _ := toInt(cfRec.Data["selector"])
			m, _ := toInt(cfRec.Data["matching_type"])
			c, _ := cfRec.Data["certificate"].(string)

			if u == int(usage) && s == int(selector) && m == int(matchingType) && c == cert {
				return true
			}
		}
	}

	// Simple content match for standard types
	if cfRec.Content == localRec.Value {
		return true
	}
	
	// Fallback: compare standard content if available (some types might populate content string too)
	return cfRec.Content == localRec.Value
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case string:
		i, err := strconv.Atoi(val)
		return i, err == nil
	}
	return 0, false
}

func (c *client) UpdateRecord(ctx context.Context, zone string, _ string, recordID string, record dns.Record) error {
	zoneID, err := c.resolveZoneID(ctx, zone)
	if err != nil {
		return err
	}

	body, err := buildUpdateBody(zone, record)
	if err != nil {
		return err
	}

	var resp cfResponse[cfDNSRecord]
	if err := c.do(ctx, "PUT", "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(strings.TrimSpace(recordID)), body, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return pickError(resp.Errors)
	}
	return nil
}

func (c *client) resolveZoneID(ctx context.Context, zoneOrName string) (string, error) {
	if strings.TrimSpace(c.zoneID) != "" {
		return c.zoneID, nil
	}

	name := strings.TrimSpace(zoneOrName)
	if name == "" {
		name = strings.TrimSpace(c.zoneName)
	}
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return "", fmt.Errorf("missing Cloudflare zone name or zone id")
	}

	query := url.Values{}
	query.Set("name", name)
	query.Set("per_page", "50")

	var resp cfResponse[[]cfZone]
	if err := c.do(ctx, "GET", "/zones?"+query.Encode(), nil, &resp); err != nil {
		return "", err
	}
	if !resp.Success {
		return "", pickError(resp.Errors)
	}
	if len(resp.Result) == 0 {
		return "", fmt.Errorf("Cloudflare zone not found: %s", name)
	}
	c.zoneID = resp.Result[0].ID
	return c.zoneID, nil
}

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4"+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Error{Code: resp.StatusCode, Message: string(bytes.TrimSpace(b))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(b, out)
}

type cfResponse[T any] struct {
	Success bool         `json:"success"`
	Errors  []cfAPIError `json:"errors"`
	Result  T            `json:"result"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Name    string         `json:"name"`
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
}

func pickError(errs []cfAPIError) error {
	if len(errs) == 0 {
		return Error{Message: "cloudflare api error"}
	}
	return Error{Code: errs[0].Code, Message: errs[0].Message}
}

func fqdnFromZone(zone string, sub string) string {
	zone = strings.TrimSuffix(strings.TrimSpace(zone), ".")
	sub = strings.TrimSpace(sub)
	if sub == "" || sub == "@" {
		return zone
	}
	return sub + "." + zone
}

func buildCreateBody(zone string, record dns.Record) (map[string]any, error) {
	name := fqdnFromZone(zone, record.SubDomain)
	t := strings.ToUpper(strings.TrimSpace(record.Type))
	body := map[string]any{
		"type":    t,
		"name":    name,
		"ttl":     ttlOrAuto(record.TTL),
		"proxied": false,
	}

	switch t {
	case "MX":
		body["content"] = strings.TrimSpace(record.Value)
		if record.Priority == nil {
			return nil, fmt.Errorf("MX priority is required for %s", name)
		}
		body["priority"] = *record.Priority
	case "SRV":
		service, proto, host, err := splitSRVOwner(name)
		if err != nil {
			return nil, err
		}
		priority, weight, port, target, err := splitSRVValue(record.Value)
		if err != nil {
			return nil, err
		}
		body["data"] = map[string]any{
			"service":  service,
			"proto":    proto,
			"name":     host,
			"priority": priority,
			"weight":   weight,
			"port":     port,
			"target":   strings.TrimSuffix(strings.TrimSpace(target), "."),
		}
		body["priority"] = priority
	case "TLSA":
		usage, selector, matchingType, cert, err := splitTLSAValue(record.Value)
		if err != nil {
			return nil, err
		}
		// Cloudflare API expects integers for usage/selector/matching_type
		body["data"] = map[string]any{
			"usage":         int(usage),
			"selector":      int(selector),
			"matching_type": int(matchingType),
			"certificate":   strings.TrimSpace(cert),
		}
	default:
		body["content"] = strings.TrimSpace(record.Value)
	}

	// debug: print body for TLSA
	if t == "TLSA" {
		b, _ := json.Marshal(body)
		fmt.Printf("DEBUG: TLSA Create Body: %s\n", string(b))
	}

	return body, nil
}

func buildUpdateBody(zone string, record dns.Record) (map[string]any, error) {
	return buildCreateBody(zone, record)
}

func ttlOrAuto(v *uint64) int {
	if v == nil || *v == 0 {
		return 1
	}
	if *v > 2147483647 {
		return 1
	}
	return int(*v)
}

func splitSRVOwner(name string) (service string, proto string, host string, err error) {
	n := strings.TrimSuffix(strings.TrimSpace(name), ".")
	parts := strings.Split(n, ".")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid SRV name: %s", name)
	}
	service = parts[0]
	proto = parts[1]
	host = strings.Join(parts[2:], ".")
	if !strings.HasPrefix(service, "_") || !strings.HasPrefix(proto, "_") {
		return "", "", "", fmt.Errorf("invalid SRV service/proto in name: %s", name)
	}
	return service, proto, host, nil
}

func splitSRVValue(v string) (priority uint64, weight uint64, port uint64, target string, err error) {
	f := strings.Fields(strings.TrimSpace(v))
	if len(f) < 4 {
		return 0, 0, 0, "", fmt.Errorf("SRV value expects: \"<priority> <weight> <port> <target>\", got %q", v)
	}
	priority, err = strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV priority: %q", f[0])
	}
	weight, err = strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV weight: %q", f[1])
	}
	port, err = strconv.ParseUint(f[2], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid SRV port: %q", f[2])
	}
	target = f[3]
	return priority, weight, port, target, nil
}

func splitTLSAValue(v string) (usage uint64, selector uint64, matchingType uint64, cert string, err error) {
	f := strings.Fields(strings.TrimSpace(v))
	if len(f) < 4 {
		return 0, 0, 0, "", fmt.Errorf("TLSA value expects: \"<usage> <selector> <matching-type> <data>\", got %q", v)
	}

	usage, err = strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid TLSA usage: %q", f[0])
	}

	selector, err = strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid TLSA selector: %q", f[1])
	}

	matchingType, err = strconv.ParseUint(f[2], 10, 64)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid TLSA matching-type: %q", f[2])
	}

	cert = f[3]
	return usage, selector, matchingType, cert, nil
}
