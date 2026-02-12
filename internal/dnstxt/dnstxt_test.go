package dnstxt

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"ddnsjx/internal/config"
)

func TestParseMatchesExampleConfig(t *testing.T) {
	root := repoRoot(t)

	records, issues, err := LoadFile(filepath.Join(root, "dns.txt"))
	if err != nil {
		t.Fatal(err)
	}

	for _, is := range issues {
		if strings.EqualFold(is.Level, "error") {
			t.Fatalf("unexpected error issue: line=%d msg=%s", is.Line, is.Message)
		}
	}

	got := config.FileConfig{Records: records}
	want, err := config.LoadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("converted config does not match config.json")
	}
}

func TestRenderZoneContainsExpectedLines(t *testing.T) {
	root := repoRoot(t)

	records, _, err := LoadFile(filepath.Join(root, "dns.txt"))
	if err != nil {
		t.Fatal(err)
	}

	domain := config.InferDomain(records)
	zone, _, err := RenderZone(domain, records, ZoneOptions{DefaultTTL: 300})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(zone, "$ORIGIN iqwq.com.\n") {
		t.Fatalf("missing $ORIGIN line")
	}
	if !strings.Contains(zone, "@ IN MX 10 mail.iqwq.com.\n") {
		t.Fatalf("missing MX line")
	}
	if !strings.Contains(zone, "_dmarc IN TXT ") {
		t.Fatalf("missing DMARC TXT line")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
