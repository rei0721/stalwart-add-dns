package app

import (
	"context"
	"errors"
	"testing"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/dnspodclient"
)

type fakeClient struct {
	failOnIndex int
	createdIDs  []uint64
	deletedIDs  []uint64
	createCalls int
	deleteCalls int
}

func (c *fakeClient) CreateRecord(ctx context.Context, domain, recordLine string, record dns.Record) (uint64, dnspodclient.CreateStatus, error) {
	c.createCalls++
	if c.failOnIndex > 0 && c.createCalls == c.failOnIndex {
		return 0, dnspodclient.CreateStatusFail, dnspodclient.Error{Code: "InternalError", Message: "boom"}
	}
	id := uint64(1000 + c.createCalls)
	c.createdIDs = append(c.createdIDs, id)
	return id, dnspodclient.CreateStatusSuccess, nil
}

func (c *fakeClient) DeleteRecord(ctx context.Context, domain string, recordID uint64) error {
	c.deleteCalls++
	c.deletedIDs = append(c.deletedIDs, recordID)
	return nil
}

func TestRunnerRollbackOnFailure(t *testing.T) {
	client := &fakeClient{failOnIndex: 3}
	r := NewRunner(client, RunnerOptions{SleepBetween: 0, Retries: 0})

	plan := dns.Plan{
		Domain:     "example.com",
		RecordLine: "默认",
		Records: []dns.Record{
			{Type: "TXT", SubDomain: "@", Value: "a"},
			{Type: "TXT", SubDomain: "b", Value: "b"},
			{Type: "TXT", SubDomain: "c", Value: "c"},
		},
	}

	err := r.Apply(context.Background(), plan)
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr dnspodclient.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected dnspodclient.Error, got %T", err)
	}
	if len(client.createdIDs) != 2 {
		t.Fatalf("expected 2 created records, got %d", len(client.createdIDs))
	}
	if len(client.deletedIDs) != 2 {
		t.Fatalf("expected rollback of 2 records, got %d", len(client.deletedIDs))
	}
	if client.deletedIDs[0] != client.createdIDs[1] || client.deletedIDs[1] != client.createdIDs[0] {
		t.Fatalf("expected reverse-order rollback, got deleted=%v created=%v", client.deletedIDs, client.createdIDs)
	}
}
