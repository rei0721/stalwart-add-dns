package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"ddnsjx/internal/app"
	"ddnsjx/internal/config"
	"ddnsjx/internal/dnspodclient"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "convert" {
		os.Exit(runConvert(os.Args[2:]))
	}

	var (
		configPath = flag.String("config", "config.json", "path to records config JSON")
		domain     = flag.String("domain", "", "DNSPod domain (empty: infer from config)")
		region     = flag.String("region", "ap-guangzhou", "TencentCloud region")
		line       = flag.String("record-line", "默认", "DNSPod record line")
		dryRun     = flag.Bool("dry-run", false, "print planned operations without calling DNSPod")
		skipUnsup  = flag.Bool("skip-unsupported", false, "skip unsupported DNSPod record types instead of failing")
		sleep      = flag.Duration("sleep", 150*time.Millisecond, "sleep between requests")
		retries    = flag.Int("retries", 3, "max retries for transient errors")
		secretID   = flag.String("secret-id", "", "TencentCloud secret id (empty: use env DNSPOD_SECRET_ID)")
		secretKey  = flag.String("secret-key", "", "TencentCloud secret key (empty: use env DNSPOD_SECRET_KEY)")
	)

	flag.Parse()

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	resolvedDomain := *domain
	if resolvedDomain == "" {
		resolvedDomain = config.InferDomain(cfg.Records)
	}
	if resolvedDomain == "" {
		fmt.Fprintln(os.Stderr, "domain is required (flag --domain) or inferable from config")
		os.Exit(1)
	}

	plan, err := config.BuildPlan(resolvedDomain, *line, cfg.Records)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if *dryRun {
		app.PrintPlan(os.Stdout, plan)
		return
	}

	plan, err = validateOrFilterPlanForDNSPod(plan, *skipUnsup)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	resolvedSecretID := *secretID
	if resolvedSecretID == "" {
		resolvedSecretID = os.Getenv("DNSPOD_SECRET_ID")
	}
	resolvedSecretKey := *secretKey
	if resolvedSecretKey == "" {
		resolvedSecretKey = os.Getenv("DNSPOD_SECRET_KEY")
	}
	if resolvedSecretID == "" || resolvedSecretKey == "" {
		fmt.Fprintln(os.Stderr, "missing credentials: set DNSPOD_SECRET_ID and DNSPOD_SECRET_KEY (or pass flags)")
		os.Exit(1)
	}

	client, err := dnspodclient.New(dnspodclient.NewOptions{
		SecretID:  resolvedSecretID,
		SecretKey: resolvedSecretKey,
		Region:    *region,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	runner := app.NewRunner(client, app.RunnerOptions{
		SleepBetween: *sleep,
		Retries:      *retries,
	})

	ctx := context.Background()
	if err := runner.Apply(ctx, plan); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
