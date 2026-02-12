package dnspodclient

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/provider"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

type NewOptions struct {
	SecretID  string
	SecretKey string
	Region    string
}

type client struct {
	sdk *dnspod.Client
}

type Error struct {
	Code    string
	Message string
}

func (e Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e Error) Retryable() bool {
	switch e.Code {
	case "ResourceInsufficient.OverLimit", "RequestLimitExceeded", "InternalError", "InternalError.Unknown":
		return true
	default:
		return false
	}
}

func IsRetryable(err error) bool {
	var e Error
	if !AsError(err, &e) {
		return false
	}
	switch e.Code {
	case "ResourceInsufficient.OverLimit", "RequestLimitExceeded", "InternalError", "InternalError.Unknown":
		return true
	default:
		return false
	}
}

func AsError(err error, target *Error) bool {
	if err == nil || target == nil {
		return false
	}
	e, ok := err.(Error)
	if !ok {
		return false
	}
	*target = e
	return true
}

func New(opt NewOptions) (provider.Client, error) {
	if opt.SecretID == "" || opt.SecretKey == "" {
		return nil, fmt.Errorf("missing credentials")
	}
	if opt.Region == "" {
		opt.Region = "ap-guangzhou"
	}

	cred := common.NewCredential(opt.SecretID, opt.SecretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "dnspod.tencentcloudapi.com"
	sdk, err := dnspod.NewClient(cred, opt.Region, cpf)
	if err != nil {
		return nil, fmt.Errorf("create dnspod client: %w", err)
	}
	return &client{sdk: sdk}, nil
}

func (c *client) IsSupportedRecordType(t string) bool {
	return IsSupportedRecordType(t)
}

func (c *client) CreateRecord(ctx context.Context, domain, recordLine string, record dns.Record) (string, provider.CreateStatus, error) {
	req := dnspod.NewCreateRecordRequest()
	req.Domain = common.StringPtr(domain)
	req.RecordType = common.StringPtr(record.Type)
	req.RecordLine = common.StringPtr(recordLine)
	req.Value = common.StringPtr(record.Value)
	req.SubDomain = common.StringPtr(record.SubDomain)
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

	resp, err := c.sdk.CreateRecord(req)
	if err == nil {
		return strconv.FormatUint(*resp.Response.RecordId, 10), provider.CreateStatusSuccess, nil
	}

	if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
		switch sdkErr.Code {
		case "InvalidParameter.RecordExists":
			return "", provider.CreateStatusExists, nil
		default:
			return "", provider.CreateStatusFail, Error{Code: sdkErr.Code, Message: sdkErr.Message}
		}
	}

	return "", provider.CreateStatusFail, err
}

func (c *client) DeleteRecord(ctx context.Context, domain string, recordID string) error {
	id, err := strconv.ParseUint(strings.TrimSpace(recordID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid record id %q: %w", recordID, err)
	}
	req := dnspod.NewDeleteRecordRequest()
	req.Domain = common.StringPtr(domain)
	req.RecordId = common.Uint64Ptr(id)
	if ctx != nil {
		req.SetContext(ctx)
	}
	_, err = c.sdk.DeleteRecord(req)
	return err
}

func SleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
