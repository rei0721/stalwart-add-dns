package dnspodclient

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ddnsjx/internal/dns"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

func (c *client) FindRecord(ctx context.Context, domain string, recordLine string, record dns.Record) (string, bool, error) {
	req := dnspod.NewDescribeRecordListRequest()
	req.Domain = common.StringPtr(domain)
	req.Subdomain = common.StringPtr(record.SubDomain)
	req.RecordType = common.StringPtr(record.Type)
	if ctx != nil {
		req.SetContext(ctx)
	}

	resp, err := c.sdk.DescribeRecordList(req)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return "", false, Error{Code: sdkErr.Code, Message: sdkErr.Message}
		}
		return "", false, err
	}

	if resp == nil || resp.Response == nil || len(resp.Response.RecordList) == 0 {
		return "", false, nil
	}

	var matches []*dnspod.RecordListItem
	for _, it := range resp.Response.RecordList {
		if it == nil {
			continue
		}
		if it.Line != nil && recordLine != "" && strings.TrimSpace(*it.Line) != strings.TrimSpace(recordLine) {
			continue
		}
		matches = append(matches, it)
	}

	if len(matches) == 0 {
		return "", false, nil
	}
	if len(matches) > 1 {
		return "", false, fmt.Errorf("multiple existing records found for %s %s line=%s; cannot safely update", record.Type, record.SubDomain, recordLine)
	}
	if matches[0].RecordId == nil {
		return "", false, fmt.Errorf("existing record missing id for %s %s", record.Type, record.SubDomain)
	}
	return strconv.FormatUint(*matches[0].RecordId, 10), true, nil
}

func (c *client) UpdateRecord(ctx context.Context, domain string, recordLine string, recordID string, record dns.Record) error {
	id, err := strconv.ParseUint(strings.TrimSpace(recordID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid record id %q: %w", recordID, err)
	}

	req := dnspod.NewModifyRecordRequest()
	req.Domain = common.StringPtr(domain)
	req.RecordId = common.Uint64Ptr(id)
	req.SubDomain = common.StringPtr(record.SubDomain)
	req.RecordType = common.StringPtr(record.Type)
	req.RecordLine = common.StringPtr(recordLine)
	req.Value = common.StringPtr(record.Value)
	if record.Remark != "" {
		req.Remark = common.StringPtr(record.Remark)
	}
	if record.TTL != nil {
		req.TTL = common.Uint64Ptr(*record.TTL)
	}
	if record.Type == "MX" && record.Priority != nil {
		req.MX = common.Uint64Ptr(*record.Priority)
	}
	if ctx != nil {
		req.SetContext(ctx)
	}

	_, err = c.sdk.ModifyRecord(req)
	if err == nil {
		return nil
	}
	if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
		return Error{Code: sdkErr.Code, Message: sdkErr.Message}
	}
	return err
}
