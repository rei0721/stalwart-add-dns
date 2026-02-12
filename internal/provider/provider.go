package provider

import (
	"context"

	"ddnsjx/internal/dns"
)

type CreateStatus string

const (
	CreateStatusSuccess CreateStatus = "success"
	CreateStatusExists  CreateStatus = "exists"
	CreateStatusFail    CreateStatus = "fail"
)

type Client interface {
	CreateRecord(ctx context.Context, zone string, recordLine string, record dns.Record) (recordID string, status CreateStatus, err error)
	DeleteRecord(ctx context.Context, zone string, recordID string) error
	FindRecord(ctx context.Context, zone string, recordLine string, record dns.Record) (recordID string, found bool, err error)
	UpdateRecord(ctx context.Context, zone string, recordLine string, recordID string, record dns.Record) error
	IsSupportedRecordType(t string) bool
}

