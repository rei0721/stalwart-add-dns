package app

import (
	"context"
	"testing"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/provider"
)

type fakeClient struct {
	failOnIndex int
	createdIDs  []string
	deletedIDs  []string
	createCalls int
	deleteCalls int
	findCalls   int
	updateCalls int
}

func (c *fakeClient) IsSupportedRecordType(t string) bool {
	return true
}

func (c *fakeClient) CreateRecord(ctx context.Context, domain, recordLine string, record dns.Record) (string, provider.CreateStatus, error) {
	c.createCalls++
	if c.failOnIndex > 0 && c.createCalls == c.failOnIndex {
		return "", provider.CreateStatusFail, fakeRetryableError{}
	}
	id := "id-" + string(rune('0'+c.createCalls))
	c.createdIDs = append(c.createdIDs, id)
	return id, provider.CreateStatusSuccess, nil
}

func (c *fakeClient) DeleteRecord(ctx context.Context, domain string, recordID string) error {
	c.deleteCalls++
	c.deletedIDs = append(c.deletedIDs, recordID)
	return nil
}

func (c *fakeClient) FindRecord(ctx context.Context, zone string, recordLine string, record dns.Record) (string, bool, error) {
	c.findCalls++
	if len(c.createdIDs) == 0 {
		return "", false, nil
	}
	return c.createdIDs[len(c.createdIDs)-1], true, nil
}

func (c *fakeClient) UpdateRecord(ctx context.Context, zone string, recordLine string, recordID string, record dns.Record) error {
	c.updateCalls++
	return nil
}

type fakeRetryableError struct{}

func (fakeRetryableError) Error() string   { return "boom" }
func (fakeRetryableError) Retryable() bool { return false }

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

type upsertClient struct {
	updateCalled bool
}

func (c *upsertClient) IsSupportedRecordType(t string) bool { return true }

func (c *upsertClient) CreateRecord(ctx context.Context, zone string, recordLine string, record dns.Record) (string, provider.CreateStatus, error) {
	return "", provider.CreateStatusExists, nil
}

func (c *upsertClient) DeleteRecord(ctx context.Context, zone string, recordID string) error {
	return nil
}

func (c *upsertClient) FindRecord(ctx context.Context, zone string, recordLine string, record dns.Record) (string, bool, error) {
	return "existing-id", true, nil
}

func (c *upsertClient) UpdateRecord(ctx context.Context, zone string, recordLine string, recordID string, record dns.Record) error {
	c.updateCalled = true
	return nil
}

func TestRunnerUpsertUpdatesOnExists(t *testing.T) {
	client := &upsertClient{}
	r := NewRunner(client, RunnerOptions{SleepBetween: 0, Retries: 0, Upsert: true})

	plan := dns.Plan{
		Domain:     "example.com",
		RecordLine: "默认",
		Records: []dns.Record{
			{Type: "TXT", SubDomain: "@", Value: "a"},
		},
	}

	if err := r.Apply(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !client.updateCalled {
		t.Fatalf("expected update to be called")
	}
}
