package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"ddnsjx/internal/dns"
	"ddnsjx/internal/provider"
)

type RunnerOptions struct {
	SleepBetween time.Duration
	Retries      int
	Upsert       bool
}

type Runner struct {
	client provider.Client
	opt    RunnerOptions
}

func NewRunner(client provider.Client, opt RunnerOptions) *Runner {
	if opt.Retries < 0 {
		opt.Retries = 0
	}
	if opt.SleepBetween < 0 {
		opt.SleepBetween = 0
	}
	return &Runner{client: client, opt: opt}
}

func PrintPlan(w io.Writer, plan dns.Plan) {
	fmt.Fprintf(w, "Domain: %s\n", plan.Domain)
	fmt.Fprintf(w, "RecordLine: %s\n", plan.RecordLine)
	fmt.Fprintf(w, "Records: %d\n", len(plan.Records))
	fmt.Fprintln(w, strings.Repeat("-", 72))
	for _, r := range plan.Records {
		if r.Priority != nil {
			fmt.Fprintf(w, "%s\t%s\tprio=%d\t%s\t%s\n", r.Type, r.SubDomain, *r.Priority, r.Value, r.Remark)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Type, r.SubDomain, r.Value, r.Remark)
	}
}

func (r *Runner) Apply(ctx context.Context, plan dns.Plan) error {
	var created []string

	fmt.Printf("Plan: domain=%s records=%d\n", plan.Domain, len(plan.Records))
	fmt.Println(strings.Repeat("-", 72))

	for _, rec := range plan.Records {
		prefix := fmt.Sprintf("[%s] %s", rec.Type, rec.SubDomain)
		if len(prefix) < 30 {
			prefix += strings.Repeat(" ", 30-len(prefix))
		}
		fmt.Printf("%s ... ", prefix)

		id, action, err := r.applyOneWithRetry(ctx, plan.Domain, plan.RecordLine, rec)
		if err != nil {
			fmt.Printf("failed: %s\n", err.Error())
			fmt.Println("rollback...")
			r.rollback(ctx, plan.Domain, created)
			return err
		}

		switch action {
		case "created":
			fmt.Printf("OK (ID: %s)\n", id)
			created = append(created, id)
		case "exists":
			fmt.Println("exists (skip)")
		case "updated":
			fmt.Printf("updated (ID: %s)\n", id)
		default:
			fmt.Println("OK")
		}

		if r.opt.SleepBetween > 0 {
			time.Sleep(r.opt.SleepBetween)
		}
	}

	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("done")
	return nil
}

func (r *Runner) applyOneWithRetry(ctx context.Context, domain, recordLine string, rec dns.Record) (recordID string, action string, err error) {
	attempts := 1
	if r.opt.Retries > 0 {
		attempts += r.opt.Retries
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		id, status, err := r.client.CreateRecord(ctx, domain, recordLine, rec)
		if err == nil && status == provider.CreateStatusSuccess {
			return id, "created", nil
		}
		if err == nil && status == provider.CreateStatusExists {
			if !r.opt.Upsert {
				return "", "exists", nil
			}
			existingID, found, findErr := r.client.FindRecord(ctx, domain, recordLine, rec)
			if findErr != nil {
				return "", "", findErr
			}
			if !found || existingID == "" {
				return "", "", fmt.Errorf("record exists but cannot locate record id for update: %s %s", rec.Type, rec.SubDomain)
			}
			if updErr := r.client.UpdateRecord(ctx, domain, recordLine, existingID, rec); updErr != nil {
				return "", "", updErr
			}
			return existingID, "updated", nil
		}
		if err == nil {
			return "", "", fmt.Errorf("unexpected create status: %s", status)
		}
		if !isRetryable(err) {
			return "", "", err
		}
		lastErr = err
		backoff := time.Duration(250*(i+1)) * time.Millisecond
		_ = sleepWithContext(ctx, backoff)
	}

	return "", "", lastErr
}

func (r *Runner) rollback(ctx context.Context, domain string, ids []string) {
	if len(ids) == 0 {
		return
	}

	fmt.Printf("reverting %d records...\n", len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		_ = r.client.DeleteRecord(ctx, domain, id)
		fmt.Printf("reverted %s\n", id)
		_ = sleepWithContext(ctx, r.opt.SleepBetween)
	}
}

type retryable interface {
	Retryable() bool
}

func isRetryable(err error) bool {
	var r retryable
	return errors.As(err, &r) && r.Retryable()
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
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
