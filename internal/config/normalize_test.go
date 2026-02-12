package config

import "testing"

func TestInferDomain(t *testing.T) {
	got := InferDomain([]RawRecord{{Name: "_smtp._tls.iqwq.com."}, {Name: "iqwq.com."}})
	if got != "iqwq.com" {
		t.Fatalf("expected iqwq.com, got %q", got)
	}
}

func TestToSubDomain(t *testing.T) {
	sub, err := toSubDomain("iqwq.com", "iqwq.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sub != "@" {
		t.Fatalf("expected @, got %q", sub)
	}

	sub, err = toSubDomain("iqwq.com", "mail.iqwq.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sub != "mail" {
		t.Fatalf("expected mail, got %q", sub)
	}
}

func TestParseSRV(t *testing.T) {
	p, w, po, target, err := parseSRV(RawRecord{Contents: "0 1 443 mail.iqwq.com."})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p != 0 || w != 1 || po != 443 || target != "mail.iqwq.com." {
		t.Fatalf("unexpected parsed values: %d %d %d %q", p, w, po, target)
	}
}

func TestParseMX(t *testing.T) {
	p, exch, err := parseMX(RawRecord{Contents: "10 mail.iqwq.com."})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p != 10 || exch != "mail.iqwq.com." {
		t.Fatalf("unexpected parsed values: %d %q", p, exch)
	}
}

func TestNormalizeRecordSRVValue(t *testing.T) {
	rec, err := normalizeRecord("iqwq.com", RawRecord{Type: "SRV", Name: "_jmap._tcp.iqwq.com.", Contents: "0 1 443 mail.iqwq.com."})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rec.Type != "SRV" {
		t.Fatalf("expected SRV, got %q", rec.Type)
	}
	if rec.SubDomain != "_jmap._tcp" {
		t.Fatalf("unexpected subdomain: %q", rec.SubDomain)
	}
	if rec.Value != "0 1 443 mail.iqwq.com." {
		t.Fatalf("unexpected value: %q", rec.Value)
	}
	if rec.Priority != nil {
		t.Fatalf("SRV should not use Priority/MX field")
	}
}
