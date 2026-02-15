package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ddnsjx/internal/app"
	"ddnsjx/internal/cloudflareclient"
	"ddnsjx/internal/config"
	"ddnsjx/internal/dnspodclient"
	"ddnsjx/internal/provider"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "convert" {
		os.Exit(runConvert(os.Args[2:]))
	}

	var (
		providerName = flag.String("provider", "dnspod", "dns provider: dnspod|cloudflare")
		configPath   = flag.String("config", "config.json", "path to records config JSON")
		domain       = flag.String("domain", "", "domain/zone name (empty: infer from config)")
		line         = flag.String("record-line", "默认", "DNSPod record line (ignored by cloudflare)")
		dryRun       = flag.Bool("dry-run", false, "print planned operations without calling provider API")
		upsert       = flag.Bool("upsert", false, "if record exists, update it to match current config")
		skipUnsup    = flag.Bool("skip-unsupported", false, "skip unsupported record types instead of failing")
		initCfg      = flag.Bool("init", false, "initialize config.json from dns.txt and exit")
		dnsTxtPath   = flag.String("dns-txt", "dns.txt", "path to dns.txt (for --init)")
		force        = flag.Bool("force", false, "overwrite output file(s) for --init/convert")
		sleep        = flag.Duration("sleep", 150*time.Millisecond, "sleep between requests")
		retries      = flag.Int("retries", 3, "max retries for transient errors")

		region    = flag.String("region", "ap-guangzhou", "TencentCloud region (dnspod only)")
		secretID  = flag.String("secret-id", "", "TencentCloud secret id (empty: use env DNSPOD_SECRET_ID)")
		secretKey = flag.String("secret-key", "", "TencentCloud secret key (empty: use env DNSPOD_SECRET_KEY)")

		cfToken  = flag.String("cf-token", "", "Cloudflare API token (empty: use env CLOUDFLARE_API_TOKEN)")
		cfZoneID = flag.String("cf-zone-id", "", "Cloudflare zone id (optional, empty: query by zone name)")
		replace  = flag.String("replace-target", "", "replace value/target in records, format: old=new (for --init)")
	)

	flag.Parse()

	if *initCfg {
		if err := initConfigFromDNSTxt(*dnsTxtPath, *configPath, *force, *replace); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "ok")
		return
	}

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

	var client provider.Client
	switch strings.ToLower(strings.TrimSpace(*providerName)) {
	case "dnspod":
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
		client, err = dnspodclient.New(dnspodclient.NewOptions{
			SecretID:  resolvedSecretID,
			SecretKey: resolvedSecretKey,
			Region:    *region,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	case "cloudflare":
		token := strings.TrimSpace(*cfToken)
		if token == "" {
			token = strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
		}
		client, err = cloudflareclient.New(cloudflareclient.NewOptions{
			APIToken: token,
			ZoneID:   strings.TrimSpace(*cfZoneID),
			ZoneName: resolvedDomain,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unsupported provider: "+*providerName)
		os.Exit(1)
	}

	plan, err = validateOrFilterPlan(plan, client, *skipUnsup)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	runner := app.NewRunner(client, app.RunnerOptions{
		SleepBetween: *sleep,
		Retries:      *retries,
		Upsert:       *upsert,
	})

	ctx := context.Background()
	if err := runner.Apply(ctx, plan); err != nil {
		if strings.Contains(err.Error(), "dial tcp") || strings.Contains(err.Error(), "lookup") {
			fmt.Fprintln(os.Stderr, "\n[Network Error] Connection failed. Please check your network settings or set HTTP_PROXY/HTTPS_PROXY environment variables.")
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
