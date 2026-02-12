package dnspodclient

import (
	"context"
	"fmt"
	"time"

	"ddnsjx/internal/dns"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	dnspod "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod/v20210323"
)

type CreateStatus string

const (
	CreateStatusSuccess CreateStatus = "success"
	CreateStatusExists  CreateStatus = "exists"
	CreateStatusFail    CreateStatus = "fail"
)

type Client interface {
	CreateRecord(ctx context.Context, domain, recordLine string, record dns.Record) (uint64, CreateStatus, error)
	DeleteRecord(ctx context.Context, domain string, recordID uint64) error
}

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

func New(opt NewOptions) (Client, error) {
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

func (c *client) CreateRecord(ctx context.Context, domain, recordLine string, record dns.Record) (uint64, CreateStatus, error) {
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
		return *resp.Response.RecordId, CreateStatusSuccess, nil
	}

	if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
		switch sdkErr.Code {
		case "InvalidParameter.RecordExists":
			return 0, CreateStatusExists, nil
		default:
			return 0, CreateStatusFail, Error{Code: sdkErr.Code, Message: sdkErr.Message}
		}
	}

	return 0, CreateStatusFail, err
}

func (c *client) DeleteRecord(ctx context.Context, domain string, recordID uint64) error {
	req := dnspod.NewDeleteRecordRequest()
	req.Domain = common.StringPtr(domain)
	req.RecordId = common.Uint64Ptr(recordID)
	if ctx != nil {
		req.SetContext(ctx)
	}
	_, err := c.sdk.DeleteRecord(req)
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
